package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"time"
	"unicode"

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
		ServerVersion: "SSH-2.0-OpenSSH_7.2p2 Ubuntu-4",
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
	log.Print("Listening on 22 PORT")

	for {

		timeoutSec := 10

		isFirst := true

		tcpConn, err := listener.Accept()
		if err != nil {
			log.Println("Listener accept failed:", err)
			continue
		}
		defer tcpConn.Close()

		go func() {

			tcpTimeout := make(chan string, 1)

			go func() {

				sshTimeout := make(chan string, 1)

				sshConn, channels, requests, err := ssh.NewServerConn(tcpConn, serverConfig)
				if err != nil {
					log.Println("New server connect failed:", err)
					err = tcpConn.Close()
					if err != nil {
						log.Println(err)
					}
					return
				}
				defer sshConn.Close()

				tcpTimeout <- "TCP No Timeout"

				utcTime := time.Now().UTC().Format(time.RFC3339Nano)

				fileName := "./log/" + utcTime + ".log"

				file, err := os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE, 0666)
				if err != nil {
					log.Fatal("Failed open file:", err)
				}
				defer file.Close()

				fmt.Fprint(file, "RemoteAddr:"+sshConn.RemoteAddr().String()+"\n")
				fmt.Fprint(file, "User:"+string(sshConn.User())+"\n")
				fmt.Fprint(file, "Password:"+password+"\n")
				fmt.Fprint(file, "ServerVersion:"+string(sshConn.ServerVersion())+"\n")
				fmt.Fprint(file, "ClientVersion:"+string(sshConn.ClientVersion())+"\n")
				fmt.Fprint(file, "Time:"+utcTime+"\n")

				log.Print("New SSH connection from " + sshConn.RemoteAddr().String() + ", " + string(sshConn.ClientVersion()) + "\r\n")

				go ssh.DiscardRequests(requests)

				go func() {
					for newChannel := range channels {
						err := handleChannel(newChannel, file, sshConn.User(), isFirst)
						if err != nil {
							log.Print("HandleChannel Error :", err)
							err = sshConn.Close()
							if err != nil {
								log.Println(err)
							}
							err = tcpConn.Close()
							if err != nil {
								log.Println(err)
							}
							return
						}
					}

					isFirst = false

					sshTimeout <- "SSH No Timeout"

				}()

				select {
				case <-sshTimeout:
				case <-time.After(time.Duration(timeoutSec) * time.Second):
					err = sshConn.Close()
					if err != nil {
						log.Println(err)
					}
					err = tcpConn.Close()
					if err != nil {
						log.Println(err)
					}
					return
				}

			}()

			select {
			case <-tcpTimeout:
			case <-time.After(time.Duration(timeoutSec) * time.Second):
				tcpConn.Close()
				err := tcpConn.Close()
				if err != nil {
					log.Println(err)
				}
				return
			}

		}()

	}
}

func handleChannel(newChannel ssh.NewChannel, file *os.File, user string, isFirst bool) error {

	channelType := newChannel.ChannelType()
	fmt.Fprint(file, "ChannelType:"+channelType+"\n")

	switch channelType {
	case "direct-tcpip":
		extraData := newChannel.ExtraData()
		fmt.Fprint(file, "ExtraData:"+string(extraData)+"\n")
		errMsg := fmt.Sprintf("Forbidden channel type: %s", channelType)
		log.Print(errMsg + "\r\n")
		err := newChannel.Reject(ssh.UnknownChannelType, errMsg)
		if err != nil {
			log.Print("Reject Failed:", err.Error()+"\r\n")
			return err
		}
		return nil
	case "session":
		channel, requests, err := newChannel.Accept()
		if err != nil {
			errMsg := fmt.Sprintf("ConnectionFailed: because of %s", err.Error())
			log.Print(errMsg + "\r\n")
			err := newChannel.Reject(ssh.ConnectionFailed, errMsg)
			if err != nil {
				log.Print("Reject Failed:", err.Error()+"\r\n")
				return err
			}
			return nil
		}

		defer channel.Close()

		for req := range requests {
			if req.Type == "shell" {
				fmt.Fprint(file, "RequestTyped:Shell"+"\r\n\n\n")

				err := handleShell(channel, req, file, user, isFirst)

				if err != nil {
					log.Print("Handle Shell Error:", err.Error()+"\r\n")
					return err
				}

				return nil

			} else if req.Type == "pty-req" {

			} else if req.Type == "exec" {
				fmt.Fprint(file, "RequestTyped:Exec"+"\r\n\n\n")
				handleExec(channel, req, file, user)
				err := channel.Close()
				if err != nil {
					log.Print("Channel Close Failed:", err.Error()+"\r\n")
					return err
				}
				return nil
			} else {
				log.Print("Unknown ssh request type:", req.Type+"\r\n")
			}
		}
	default:
		errMsg := fmt.Sprintf("Unknown channel type: %s", channelType)
		log.Print(errMsg + "\r\n")
		err := newChannel.Reject(ssh.UnknownChannelType, errMsg)
		if err != nil {
			log.Print("Reject Failed:", err.Error()+"\r\n")
			return err
		}
		return errors.New(errMsg)
	}

	errMsg := "Unknown ChannelType"
	return errors.New(errMsg)
}

func handleShell(c ssh.Channel, r *ssh.Request, file *os.File, user string, isFirst bool) error {

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
			log.Print("Read EOF", "\r\n")
			return nil
		}
		if err != nil {
			log.Print("Read Line Failed:", err.Error()+"\r\n")
			return err
		}
		if line == "" {
			fmt.Fprint(term, line)
			fmt.Fprint(file, lineLabel+line+"\r\n")
			continue
		}

		emulateCommand([]byte(line), lineLabel, term, file)

	}
}

func emulateCommand(v []byte, lineLabel string, term *terminal.Terminal, file *os.File) error {
	v = bytes.TrimFunc(v, unicode.IsControl)
	splitPayload := bytes.Split(v, []byte{32})

	commandName := ""
	commandArgs := []string{}
	for _, v := range splitPayload {
		if len(v) != 0 {
			if len(commandName) == 0 {
				commandName = string(v)
			} else {
				commandArgs = append(commandArgs, string(v))
			}
		}
	}
	command := commandName + " "

	for _, v := range commandArgs {
		command += v
	}

	if commandName == "uname" {
		if len(commandArgs) > 0 {
			if commandArgs[0] == "-a" {
				msg := "Linux ubuntu 4.4.0-127-generic #153-Ubuntu SMP Sat May 19 10:58:46 UTC 2018 x86_64 x86_64 x86_64 GNU/Linux\r\n"
				fmt.Fprint(term, msg)
				fmt.Fprint(file, lineLabel+command+"\r\n")
				fmt.Fprint(file, msg)
			}
		} else {
			msg := "Linux\r\n"
			fmt.Fprint(term, msg)
			fmt.Fprint(file, lineLabel+command+"\r\n")
			fmt.Fprint(file, msg)
		}
	} else {
		fmt.Fprint(term, command+"\r\n")
		fmt.Fprint(file, lineLabel+command+"\r\n")
		fmt.Fprint(file, command)
	}

	return nil
}

func commandsFunc(lineLabel string, commandName string, commandArgs []string, term *terminal.Terminal, file *os.File) {

}

func handleExec(c ssh.Channel, r *ssh.Request, file *os.File, user string) {
	term := terminal.NewTerminal(c, "")

	lineLabel := user + "@ubuntu:~$ "

	term.SetPrompt(lineLabel + string(term.Escape.Reset))

	emulateCommand(r.Payload, lineLabel, term, file)
}
