package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"os"
	"time"
	"unicode"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/terminal"
)

const (
	Ubuntu      = "Ubuntu"
	KaliLinux   = "KaliLinux"
	RaspberryPi = "RaspberryPi"
	AmazonLinux = "AmazonLinux"
	CentOS      = "CentOS"
	Debian      = "Debian"
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

	serverConfig.Ciphers = []string{
		"aes128-cbc",
		"blowfish-cbc",
		"3des-cbc",
		"aes128-gcm@openssh.com",
		"chacha20-poly1305@openssh.com",
		"aes128-ctr",
		"aes192-ctr",
		"aes256-ctr",
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

	listener, err := net.Listen("tcp", "0.0.0.0:2222")
	if err != nil {
		log.Fatalf("Failed to listen on 2222 (%s)", err)
	}
	log.Print("Listening on 2222 PORT")

	for {

		timeoutSec := 10000

		isFirst := true

		tcpConn, err := listener.Accept()
		if err != nil {
			log.Println("Listener accept failed:", err)
			continue
		}
		// defer tcpConn.Close()

		go func() {

			tcpTimeout := make(chan string, 1)

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

			log.Print("New SSH connection from " + sshConn.RemoteAddr().String() + ", " + string(sshConn.ClientVersion()) + "\n")

			go ssh.DiscardRequests(requests)

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
		log.Print(errMsg + "\n")
		err := newChannel.Reject(ssh.UnknownChannelType, errMsg)
		if err != nil {
			log.Print("Reject Failed:", err.Error()+"\n")
			return err
		}
		return nil
	case "session":
		fmt.Println("sesstioon")
		channel, requests, err := newChannel.Accept()
		if err != nil {
			errMsg := fmt.Sprintf("ConnectionFailed: because of %s", err.Error())
			log.Print(errMsg + "\n")
			err := newChannel.Reject(ssh.ConnectionFailed, errMsg)
			if err != nil {
				log.Print("Reject Failed:", err.Error()+"\n")
				return err
			}
			return nil
		}

		defer channel.Close()

		osVersions := []string{Ubuntu, KaliLinux, RaspberryPi, AmazonLinux, CentOS, Debian}

		rand.Seed(time.Now().UnixNano())

		os := osVersions[rand.Intn(len(osVersions))]

		fmt.Fprint(file, "OS:"+os+"\n")

		for req := range requests {
			if req.Type == "shell" {
				fmt.Fprint(file, "RequestTyped:Shell"+"\n\n\n")

				err := handleShell(channel, req, file, user, os, isFirst)

				if err != nil {
					log.Print("Handle Shell Error:", err.Error()+"\n")
					return err
				}

				return nil

			} else if req.Type == "pty-req" {

			} else if req.Type == "exec" {
				fmt.Fprint(file, "RequestTyped:Exec"+"\n\n\n")
				err := handleExec(channel, req, file, user, os)

				if err != nil {
					log.Print("Handle Exec Error:", err.Error()+"\n")
					return err
				}

				err = channel.Close()
				if err != nil {
					log.Print("Channel Close Failed:", err.Error()+"\n")
					return err
				}
				return nil
			} else {
				log.Print("Unknown ssh request type:", req.Type+"\n")
			}
		}
	default:
		errMsg := fmt.Sprintf("Unknown channel type: %s", channelType)
		log.Print(errMsg + "\n")
		err := newChannel.Reject(ssh.UnknownChannelType, errMsg)
		if err != nil {
			log.Print("Reject Failed:", err.Error()+"\n")
			return err
		}
		return errors.New(errMsg)
	}

	errMsg := "Unknown ChannelType"
	return errors.New(errMsg)
}

func handleShell(c ssh.Channel, r *ssh.Request, file *os.File, user string, os string, isFirst bool) error {

	term := terminal.NewTerminal(c, "")
	lineLabel := user + "@" + os + ":~$ "

	term.SetPrompt(lineLabel + string(term.Escape.Reset))

	if isFirst == true {

		var terminalHeader string

		if os == Ubuntu {
			// Ubuntu
			terminalHeader = "Linux ubuntu 2.6.20-16-generic #2 SMP Thu Jun 7 19:00:28 UTC 2007 x86_64\n\nThe programs included with the Ubuntu system are free software;\nthe exact distribution terms for each program are described in the\nindividual files in /usr/share/doc/*/copyright.\n\nUbuntu comes with ABSOLUTELY NO WARRANTY, to the extent permitted by applicable law.\n\nLast login: Mon Aug 13 01:05:46 2007 from 93.184.216.34\n"
		} else if os == KaliLinux {
			// KaliLinux
			terminalHeader = "Linux kali 4.14.71-v8 #1 SMP PREEMPT Wed Oct 31 21:41:06 UTC 2018 aarch64\n\nThe programs included with the Kali GNU/Linux system are free software;\nthe exact distribution terms for each program are described in the\nindividual files in /usr/share/doc/*/copyright.\n\nKali GNU/Linux comes with ABSOLUTELY NO WARRANTY, to the extent\npermitted by applicable law.\nLast login: Thu Feb  1 13:51:02 2018 from 93.184.216.34\n"
		} else if os == AmazonLinux {
			// AmazonLinux
			terminalHeader = "Last login: Sat Jun  1 09:34:32 2019 from 93.184.216.34\n\n__|  __|_  )\n_|  (     /   Amazon Linux 2 AMI\n___|\\___|___\n\nhttps://aws.amazon.com/amazon-linux-2/\n5 package(s) needed for security, out of 7 available\nRun \"sudo yum update\" to apply all updates.\n"
		} else if os == RaspberryPi {
			// RaspberryPi
			terminalHeader = "Linux rasPi 4.9.41-v7+ #1023 SMP Tue Aug 8 16:00:15 BST 2017 armv7l\n\nThe programs included with the Debian GNU/Linux system are free software;\nthe exact distribution terms for each program are described in the\nindividual files in /usr/share/doc/*/copyright.\n\nDebian GNU/Linux comes with ABSOLUTELY NO WARRANTY, to the extent\npermitted by applicable law.\nLast login: Wed Oct 11 18:54:03 2017 from 93.184.216.34\n"
		} else if os == Debian {
			// Debian
			terminalHeader = "Linux debian 3.6.5-x86_64 #1 SMP Sun Nov 4 12:40:43 EST 2012 x86_64\n\nThe programs included with the Debian GNU/Linux system are free software;\nthe exact distribution terms for each program are described in the\nindividual files in /usr/share/doc/*/copyright.\n\nDebian GNU/Linux comes with ABSOLUTELY NO WARRANTY, to the extent\npermitted by applicable law.\nLast login: Sat Dec 22 13:38:52 2012 from 93.184.216.34\n"
		} else if os == CentOS {
			// CentOS
			terminalHeader = "Last login: Thu Feb  1 13:51:02 2018 from 93.184.216.34\n"
		} else {
			errMsg := "Unknown OS: " + os + "\n"
			log.Print(errMsg)
			return errors.New(errMsg)
		}

		fmt.Fprint(term, terminalHeader)
		fmt.Fprint(file, terminalHeader)
	}

	for {
		line, err := term.ReadLine()
		if err == io.EOF {
			log.Print("Read EOF", "\n")
			return nil
		}
		if err != nil {
			log.Print("Read Line Failed:", err.Error()+"\n")
			return err
		}
		if line == "" {
			fmt.Fprint(term, line)
			fmt.Fprint(file, lineLabel+line+"\n")
			continue
		}

		err = emulateCommand([]byte(line), lineLabel, os, term, file)
		if err != nil {
			log.Print(err.Error() + "\n")
			return err
		}

	}
}

func emulateCommand(v []byte, lineLabel string, os string, term *terminal.Terminal, file *os.File) error {
	fmt.Println(string(v))
	v = bytes.TrimFunc(v, unicode.IsControl)
	splitPayload := bytes.Split(v, []byte{32})
	fmt.Println(string(v))

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
	fmt.Println(commandName, commandArgs)
	command := commandName + " "

	for _, v := range commandArgs {
		command += v
	}

	fmt.Fprint(file, lineLabel+string(v)+"\n")
	msg := command
	if commandName == "uname" {
		if len(commandArgs) > 0 {
			if commandArgs[0] == "-a" {
				if os == Ubuntu {
					// Ubuntu
					msg = "Linux ubuntu 4.10.0-35-generic #39~16.04.1-Ubuntu SMP Wed Sep 13 09:02:42 UTC 2017 x86_64 GNU/Linux\n"
				} else if os == KaliLinux {
					// KaliLinux
					msg = "Linux kali 4.14.71-v8 #1 SMP PREEMPT Wed Oct 31 21:41:06 UTC 2018 aarch64 GNU/Linux\n"
				} else if os == AmazonLinux {
					// AmazonLinux
					msg = "Linux ip-170-31-81-10.ec2.internal 4.10.109-90.92.amzn2.x86_64 #1 SMP Mon Apr 1 23:00:38 UTC 2019 x86_64 x86_64 x86_64 GNU/Linux\n"
				} else if os == RaspberryPi {
					// RaspberryPi
					msg = "Linux raspberrypi 3.18.11-v7+ #781 SMP PREEMPT Tue Apr 21 18:07:59 BST 2015 armv7l GNU/Linux\n"
				} else if os == Debian {
					// Debian
					msg = "Linux debian 3.2.0-4-amd64 #1 SMP Debian 3.2.65-1+deb7u2 x86_64 GNU/Linux\n"
				} else if os == CentOS {
					// CentOS
					msg = "Linux cent 3.10.0-327.28.2.el7.x86_64 #1 SMP Wed Aug 3 11:11:39 UTC 2016 x86_64 x86_64 x86_64 GNU/Linux\n"
				} else {
					errMsg := "Unknown OS: " + os + "\n"
					log.Print(errMsg)
					return errors.New(errMsg)
				}
			}
		} else {
			msg = "Linux\n"
		}
		fmt.Fprint(term, msg)
		fmt.Fprint(file, msg)
	} else if commandName == "/ip" {
		if len(commandArgs) > 0 {
			if len(commandArgs) == 2 {
				if commandArgs[0] == "cloud" {
					if commandArgs[1] == "print" {
						msg = "         ddns-enabled: yes\n ddns-update-interval: none\n          update-time: yes\n       public-address: 93.184.216.34\n  public-address-ipv6: 2b02:610:7501:2000::2\n             dns-name: 529c0491d41c.sn.example.net\n               status: updated\n"
					} else {
						msg = ""
					}
				} else {
					msg = ""
				}
			} else {
				msg = ""
			}
		} else {
			msg = ""
		}
		fmt.Fprint(term, msg)
		fmt.Fprint(file, msg)
	} else {
		msg = string(v) + "\n"
		fmt.Fprint(term, msg)
		fmt.Fprint(file, msg)
	}

	return nil
}

func handleExec(c ssh.Channel, r *ssh.Request, file *os.File, user string, os string) error {
	term := terminal.NewTerminal(c, "")

	lineLabel := user + "@" + os + ":~$ "

	term.SetPrompt(lineLabel + string(term.Escape.Reset))

	err := emulateCommand(r.Payload, lineLabel, os, term, file)
	if err != nil {
		log.Print(err.Error() + "\n")
		return err
	}
	return nil
}
