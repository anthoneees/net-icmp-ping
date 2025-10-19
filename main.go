package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"os"
	"syscall"
	"time"
)

type IPHeader struct {
	VersionAndIHL          uint8
	TypeOfService          uint8
	TotalLength            uint16
	Identification         uint16
	FlagsAndFragmentOffset uint16
	TimeToLive             uint8
	Protocol               uint8
	Checksum               uint16
	SourceAddress          [4]byte
	DestinationAddress     [4]byte
}

type ICMPHeader struct {
	Type           uint8
	Code           uint8
	Checksum       uint16
	Identifier     uint16
	SequenceNumber uint16
	Data           [8]byte
}

func populateIPPacket(iPStruct *IPHeader) {
	iPStruct.VersionAndIHL = 69
	iPStruct.TypeOfService = 0
	iPStruct.TotalLength = 28
	iPStruct.Identification = 0
	iPStruct.FlagsAndFragmentOffset = 0
	iPStruct.TimeToLive = 64
	iPStruct.Protocol = 1
	iPStruct.Checksum = 0
}

func populateICMPPacket(iCMPStruct *ICMPHeader) {
	iCMPStruct.Type = 8
	iCMPStruct.Code = 0
	iCMPStruct.Identifier = uint16(os.Getpid() & 0xFFFF)
	iCMPStruct.SequenceNumber = 0
	iCMPStruct.Checksum = 0
}

func checksum(data []byte) uint16 {
	var sum uint32
	for i := 0; i < len(data)-1; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(data[i : i+2]))
	}
	if len(data)%2 == 1 {
		sum += uint32(data[len(data)-1]) << 8
	}
	for (sum >> 16) > 0 {
		sum = (sum & 0xFFFF) + (sum >> 16)
	}
	return ^uint16(sum)
}

func buildPacket(ip *IPHeader, icmp *ICMPHeader) ([]byte, error) {
	icmp.Checksum = 0

	var icmpBuf bytes.Buffer
	if err := binary.Write(&icmpBuf, binary.BigEndian, icmp); err != nil {
		return nil, err
	}
	icmpBytes := icmpBuf.Bytes()

	icmp.Checksum = checksum(icmpBytes)
	icmpBuf.Reset()
	binary.Write(&icmpBuf, binary.BigEndian, icmp)
	icmpBytes = icmpBuf.Bytes()

	ip.Checksum = 0

	var ipBuf bytes.Buffer
	if err := binary.Write(&ipBuf, binary.BigEndian, ip); err != nil {
		return nil, err
	}
	ipBytes := ipBuf.Bytes()

	ip.Checksum = checksum(ipBytes)
	ipBuf.Reset()
	binary.Write(&ipBuf, binary.BigEndian, ip)
	ipBytes = ipBuf.Bytes()

	packet := append(ipBytes, icmpBytes...)
	return packet, nil
}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: go run main.go <hostname or IP>")
		return
	}
	arg := os.Args[1]

	// get local IP
	con, _ := net.Dial("udp", "8.8.8.8:80")
	localAddr := con.LocalAddr().(*net.UDPAddr)
	sourceIP := localAddr.IP
	con.Close()

	ips, err := net.LookupIP(arg)
	if err != nil {
		fmt.Println("Error resolving:", err)
		return
	}

	var destIP net.IP
	for _, ip := range ips {
		if ipv4 := ip.To4(); ipv4 != nil {
			destIP = ipv4
			break
		}
	}
	if destIP == nil {
		fmt.Println("No IPv4 address found for", arg)
		return
	}

	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_ICMP)
	if err != nil {
		fmt.Println("Error creating raw socket:", err)
		return
	}
	defer syscall.Close(fd)

	if err := syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_HDRINCL, 1); err != nil {
		fmt.Println("Failed to set IP_HDRINCL:", err)
		return
	}

	tv := syscall.Timeval{Sec: 1, Usec: 0}
	syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &tv)

	ipHeaderInst := IPHeader{SourceAddress: [4]byte(sourceIP), DestinationAddress: [4]byte(destIP)}
	icmpHeaderInst := ICMPHeader{}
	populateIPPacket(&ipHeaderInst)
	populateICMPPacket(&icmpHeaderInst)

	dstAddr := &syscall.SockaddrInet4{Port: 0, Addr: ipHeaderInst.DestinationAddress}

	fmt.Printf("Pinging %s (%s) with 32 bytes of data:\n\n", arg, net.IP(ipHeaderInst.DestinationAddress[:]))

	receivedPackets := 0
	var rtts []time.Duration

	for i := 0; i < 4; i++ {
		icmpHeaderInst.SequenceNumber++
		binary.BigEndian.PutUint64(icmpHeaderInst.Data[:], uint64(time.Now().UnixNano()))

		packet, err := buildPacket(&ipHeaderInst, &icmpHeaderInst)
		if err != nil {
			panic(err)
		}

		if err := syscall.Sendto(fd, packet, 0, dstAddr); err != nil {
			fmt.Println("Error sending packet:", err)
			continue
		}

		buf := make([]byte, 1024)
		n, from, err := syscall.Recvfrom(fd, buf, 0)
		if err != nil {
			fmt.Println("Request timed out.")
		} else if n >= 28 {
			ipHeader := buf[:20]
			icmpHeader := buf[20:n]

			if icmpHeader[0] == 0 && icmpHeader[1] == 0 { // ICMP echo reply
				receivedPackets++
				src := from.(*syscall.SockaddrInet4)
				seq := binary.BigEndian.Uint16(icmpHeader[6:8])
				sentTime := int64(binary.BigEndian.Uint64(icmpHeader[8:16]))
				elapsed := time.Since(time.Unix(0, sentTime))
				rtts = append(rtts, elapsed)

				fmt.Printf("%d bytes from %v: icmp_seq=%d ttl=%d time=%.3f ms\n",
					n-20, net.IP(src.Addr[:]), seq, ipHeader[8], float64(elapsed.Microseconds())/1000)
			}
		}

		time.Sleep(1 * time.Second)
	}

	// compute stats
	var sum, min, max time.Duration
	if len(rtts) > 0 {
		min, max = rtts[0], rtts[0]
		for _, r := range rtts {
			if r < min {
				min = r
			}
			if r > max {
				max = r
			}
			sum += r
		}
	}

	avg := time.Duration(0)
	if len(rtts) > 0 {
		avg = time.Duration(int64(sum) / int64(len(rtts)))
	}

	// mdev
	var sqDiffSum float64
	for _, r := range rtts {
		diff := float64(r - avg)
		sqDiffSum += diff * diff
	}
	mdev := time.Duration(0)
	if len(rtts) > 0 {
		mdev = time.Duration(int64(math.Sqrt(sqDiffSum / float64(len(rtts)))))
	}

	fmt.Printf("\n--- %s ping statistics ---\n", net.IP(ipHeaderInst.DestinationAddress[:]).String())
	fmt.Printf("4 packets transmitted, %d received, %.1f%% packet loss, time %dms\n",
		receivedPackets, float64(4-receivedPackets)/4*100, sum.Milliseconds())
	fmt.Printf("rtt min/avg/max/mdev = %.3f/%.3f/%.3f/%.3f ms\n",
		float64(min.Microseconds())/1000,
		float64(avg.Microseconds())/1000,
		float64(max.Microseconds())/1000,
		float64(mdev.Microseconds())/1000)
}
