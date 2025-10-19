# net-icmp-ping

A minimal implementation of the ICMP protocol written in Go.  
This project constructs raw IP and ICMP packets manually and sends echo requests using raw sockets, simulating the behavior of the `ping` utility.

---

## Concepts Covered
- Raw sockets and the `syscall` package
- IP header construction and checksum calculation
- ICMP Echo Request/Reply packet structure
- RTT (Round Trip Time) calculation and packet statistics

---

## How It Works
1. Builds an IP header and ICMP header manually.
2. Computes checksums for both layers.
3. Sends the packet using `syscall.Sendto`.
4. Waits for ICMP replies and measures latency.

---

## Example Usage
sudo go run main.go 8.8.8.8
Pinging 8.8.8.8 (8.8.8.8) with 32 bytes of data:

16 bytes from 8.8.8.8: icmp_seq=1 ttl=119 time=5.544 ms
16 bytes from 8.8.8.8: icmp_seq=2 ttl=119 time=7.947 ms
16 bytes from 8.8.8.8: icmp_seq=3 ttl=119 time=6.908 ms
16 bytes from 8.8.8.8: icmp_seq=4 ttl=119 time=11.282 ms

--- 8.8.8.8 ping statistics ---
4 packets transmitted, 4 received, 0.0% packet loss, time 31ms
rtt min/avg/max/mdev = 5.544/7.920/11.282/2.119 ms
