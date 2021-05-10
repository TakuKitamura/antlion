package main

import (
	"fmt"
	"io"
	"log"
	"net"
)

func main() {
	tcpListener, err := net.Listen("tcp", "0.0.0.0:5555")
	if err != nil {
		log.Fatalf("failed to listen on 5555 (%s)", err)
	}
	defer tcpListener.Close()

	for {

		conn, _ := tcpListener.Accept()
		defer conn.Close()

		go func() {

			// r := bufio.NewReader(conn)
			// w := bufio.NewWriter(conn)

			// rw := bufio.NewReadWriter(r, w)

			term := NewTerminal(conn, "", "TELNET")

			pwd, _ := term.ReadPassword("pass: ")

			fmt.Println(string(pwd))

			term.SetPrompt("> ")

			for {

				line, err := term.ReadLine()
				if err == io.EOF {
					log.Print("read eof", "\n")
					// return nil
				}
				if err != nil {
					log.Print("read line failed:", err.Error()+"\n")
					// return err
				}
				if line == "" {
					continue
				}

				term.Write([]byte(line + "\n"))

				// err = emulateCommand([]byte(line), lineLabel, kernelInfo, term, logFile, commandList)
				// if err != nil {
				// 	log.Print(err.Error() + "\n")
				// 	return err
				// }

			}
		}()
	}
}
