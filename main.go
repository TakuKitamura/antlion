package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/terminal"
)

func main() {
	password := ""

	serverConfig := &ssh.ServerConfig{
		// NoClientAuth: true,
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			password = string(pass)
			return nil, nil
		},
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

	for {

		isFirst := true

		utcTime := time.Now().UTC().Format(time.RFC3339Nano)

		fileName := "./log/" + utcTime + ".log"

		file, err := os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE, 0666)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()

		tcpConn, err := listener.Accept()
		if err != nil {
			log.Print(err)
			continue
		}
		defer tcpConn.Close()

		sshConn, chans, reqs, err := ssh.NewServerConn(tcpConn, serverConfig)

		if err != nil {
			log.Print(err)
			continue
		}
		defer sshConn.Close()

		fmt.Fprint(file, "RemoteAddr:"+sshConn.RemoteAddr().String()+"\n")
		fmt.Fprint(file, "User:"+string(sshConn.User())+"\n")
		fmt.Fprint(file, "Password:"+password+"\n")
		fmt.Fprint(file, "ServerVersion:"+string(sshConn.ServerVersion())+"\n")
		fmt.Fprint(file, "ClientVersion:"+string(sshConn.ClientVersion())+"\n")
		fmt.Fprint(file, "Time:"+utcTime+"\n")
		fmt.Fprint(file, "\n\n")

		log.Print("New SSH connection from " + sshConn.RemoteAddr().String() + ", " + string(sshConn.ClientVersion()) + "\r\n")

		go ssh.DiscardRequests(reqs)

		go handleChannels(chans, file, sshConn.User(), isFirst)

		isFirst = false

	}
}

func handleChannels(chans <-chan ssh.NewChannel, file *os.File, user string, isFirst bool) {
	for newChannel := range chans {
		go handleChannel(newChannel, file, user, isFirst)
	}
}

func handleChannel(newChannel ssh.NewChannel, file *os.File, user string, isFirst bool) {

	channelType := newChannel.ChannelType()
	if channelType != "session" {
		errMsg := fmt.Sprintf("Unknown channel type: %s", channelType)
		newChannel.Reject(ssh.UnknownChannelType, errMsg)
		log.Print(errMsg + "\r\n")
	}

	channel, requests, err := newChannel.Accept()
	if err != nil {
		errMsg := fmt.Sprintf("ConnectionFailed: because of %s", err.Error())
		newChannel.Reject(ssh.ConnectionFailed, errMsg)
		log.Print(errMsg + "\r\n")
	}
	defer channel.Close()

	for req := range requests {
		switch req.Type {
		case "shell":
			go handleShell(channel, req, file, user, isFirst)
		case "exec":
			handleExec(channel, req, file)
			return
		}
	}

}

func handleShell(c ssh.Channel, r *ssh.Request, file *os.File, user string, isFirst bool) {

	oldState, err := terminal.MakeRaw(0)
	if err != nil {
		log.Print(err.Error() + "\r\n")
		return
	}
	defer terminal.Restore(0, oldState)

	term := terminal.NewTerminal(c, "")
	lineLabel := user + "@ubuntu:~$ "

	term.SetPrompt(lineLabel + string(term.Escape.Reset))

	if isFirst == true {
		terminalHeader := "Welcome to Ubuntu 16.04.3 LTS (GNU/Linux 4.4.0-112-generic x86_64)\n\n * Documentation:  https://help.ubuntu.com\n * Management:     https://landscape.canonical.com\n * Support:        https://ubuntu.com/advantage\n\nLast login: Thu Feb  1 13:51:02 2018 from 93.184.216.34\n"

		fmt.Fprint(term, terminalHeader)
		fmt.Fprint(file, terminalHeader)
	}

	for {
		line, err := term.ReadLine()
		if err == io.EOF {
			c.Close()
			return
		}
		if err != nil {
			log.Print(err.Error() + "\r\n")
			c.Close()
			return
		}
		if line == "" {
			fmt.Fprint(term, line)
			fmt.Fprint(file, lineLabel+line+"\r\n")
			continue
		}
		fmt.Fprint(term, line+"\r\n")
		fmt.Fprint(file, lineLabel+line+"\r\n"+line+"\r\n")

	}
}

func handleExec(c ssh.Channel, r *ssh.Request, file *os.File) {
	oldState, err := terminal.MakeRaw(0)
	if err != nil {
		log.Print(err.Error() + "\r\n")
		return
	}
	defer terminal.Restore(0, oldState)
	term := terminal.NewTerminal(c, "")

	fmt.Fprint(term, string(r.Payload)+"\r\n")
	fmt.Fprint(file, string(r.Payload)+"\r\n")
	return
}
