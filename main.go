package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/terminal"
)

func main() {
	serverConfig := &ssh.ServerConfig{
		NoClientAuth: true,
	}

	privateKeyBytes, err := ioutil.ReadFile("id_rsa")
	if err != nil {
		log.Fatal("Failed to load private key (./id_rsa)")
	}

	privateKey, err := ssh.ParsePrivateKey(privateKeyBytes)
	if err != nil {
		log.Fatal("Failed to parse private key")
	}

	serverConfig.AddHostKey(privateKey)

	listener, err := net.Listen("tcp", "0.0.0.0:22")
	if err != nil {
		log.Fatalf("Failed to listen on 22 (%s)", err)
	}
	log.Print("Listening on 22...")

	isFirst := true

	for {
		tcpConn, err := listener.Accept()

		if err != nil {
			log.Fatalf("Failed to accept on 22 (%s)", err)
		}

		sshConn, chans, reqs, err := ssh.NewServerConn(tcpConn, serverConfig)
		fmt.Println(sshConn.RemoteAddr().String())
		if err != nil {
			log.Fatalf("Failed to handshake (%s)", err)
		}
		log.Printf("New SSH connection from %s (%s)", sshConn.RemoteAddr(), sshConn.ClientVersion())

		utcTime := time.Now().UTC().Format(time.RFC3339)

		dir := "./log/"

		fileName := dir + sshConn.RemoteAddr().String() + "|" + string(sshConn.User()) + "|" + string(sshConn.ServerVersion()) + "|" + string(sshConn.ClientVersion()) + "|" + utcTime + ".log"

		file, err := os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE, 0666)
		if err != nil {
			log.Fatal(err)
		}

		defer file.Close()

		go ssh.DiscardRequests(reqs)

		go handleChannels(chans, file, isFirst)

		isFirst = false

	}
}

func handleChannels(chans <-chan ssh.NewChannel, file *os.File, isFirst bool) {
	for newChannel := range chans {
		go handleChannel(newChannel, file, isFirst)
	}
}

func writeTerminal(t *terminal.Terminal, file *os.File, str string) error {
	_, err := t.Write([]byte(str))

	if err != nil {
		return err
	}

	fmt.Fprint(file, str)

	return nil
}

func handleChannel(newChannel ssh.NewChannel, file *os.File, isFirst bool) {
	if t := newChannel.ChannelType(); t != "session" {
		newChannel.Reject(ssh.UnknownChannelType, fmt.Sprintf("Unknown channel type: %s", t))
		return
	}

	conn, _, err := newChannel.Accept()
	if err != nil {
		log.Fatalf("Could not accept channel (%s)", err)
		return
	}

	t := terminal.NewTerminal(conn, "")

	if isFirst == true {
		terminalHeader := "Welcome to Ubuntu 16.04.3 LTS (GNU/Linux 4.4.0-112-generic x86_64)\n\n * Documentation:  https://help.ubuntu.com\n * Management:     https://landscape.canonical.com\n * Support:        https://ubuntu.com/advantage\n\nLast login: Thu Feb  1 13:51:02 2018 from 93.184.216.34\n"

		writeTerminal(t, file, terminalHeader)
	}

	lineLabel := "jacob@ubuntu:~$ "

	for {

		writeTerminal(t, file, lineLabel)

		// _, err = t.Write([]byte(lineLabel))

		input, err := t.ReadLine()

		if err != nil {
			log.Printf("failed to read: %v", err)
			return
		}

		if len(input) > 0 {
			writeTerminal(t, file, input+"\n")
		} else {
			writeTerminal(t, file, input)
		}

		if len(input) > 0 {
			fmt.Fprint(file, string(input+"\n"))
		} else {
			fmt.Fprint(file, string(input)+"\n")
		}

	}
}
