package main

import (
	"net"
	"os"
)

func main() {
	conn, err := net.Dial("tcp", "127.0.0.1:0")
	if err != nil {
		os.Exit(1)
	}
	conn.Close()
}
