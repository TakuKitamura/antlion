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
	"strings"
	"time"
	"unicode"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
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

	if _, err := os.Stat("./log"); os.IsNotExist(err) {
		os.Mkdir("./log", 0766)
	}

	privateKeyBytes, err := ioutil.ReadFile("id_rsa")
	if err != nil {
		log.Fatal("failed to load private key (./id_rsa)")
	}

	privateKey, err := ssh.ParsePrivateKey(privateKeyBytes)
	if err != nil {
		log.Fatal("failed to parse private key")
	}

	serverConfig.AddHostKey(privateKey)

	tcpListener, err := net.Listen("tcp", "0.0.0.0:2222")
	if err != nil {
		log.Fatalf("failed to listen on 2222 (%s)", err)
	}

	log.Print("listening on 2222 port")

	timeoutSec := 30

	log.Print("ssh timeout is ", timeoutSec, "s")

	for {

		tcpConn, err := tcpListener.Accept()
		if err != nil {
			log.Println("listener accept failed:", err)
			continue
		}

		go func() {

			// TODO: 二重Closeを防ぐ

			go func() {
				time.Sleep(time.Duration(timeoutSec) * time.Second)
				log.Println("timeout")
				err = tcpConn.Close()
				if err != nil {
					log.Println(err)
				}
			}()

			sshConn, sshCh, _, err := ssh.NewServerConn(tcpConn, serverConfig)
			if err != nil {
				log.Println("new server connect failed:", err)
				err = tcpConn.Close()
				if err != nil {
					log.Println(err)
				}
				return
			}

			utcTime := time.Now().UTC().Format(time.RFC3339Nano)

			logFileName := "./log/" + utcTime + ".log"

			logFile, err := os.OpenFile(logFileName, os.O_WRONLY|os.O_CREATE, 0666)
			if err != nil {
				log.Fatal("failed open log file:", err)
			}
			defer logFile.Close()

			fmt.Fprint(logFile, "RemoteAddr:"+sshConn.RemoteAddr().String()+"\n")
			fmt.Fprint(logFile, "User:"+string(sshConn.User())+"\n")
			fmt.Fprint(logFile, "Password:"+password+"\n")
			fmt.Fprint(logFile, "ServerVersion:"+string(sshConn.ServerVersion())+"\n")
			fmt.Fprint(logFile, "ClientVersion:"+string(sshConn.ClientVersion())+"\n")
			fmt.Fprint(logFile, "Time:"+utcTime+"\n")

			log.Print("new ssh connection from " + sshConn.RemoteAddr().String() + ", " + string(sshConn.ClientVersion()) + "\n")

			go func() {
				for c := range sshCh {
					go func(sshNewChannel ssh.NewChannel) {
						err := handleChannel(sshNewChannel, logFile, sshConn.User())
						if err != nil {
							log.Print("handle channel error :", err)
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
					}(c)
				}
			}()

		}()

	}
}

func handleChannel(sshNewChannel ssh.NewChannel, logFile *os.File, userName string) error {

	channelType := sshNewChannel.ChannelType()

	switch channelType {
	case "direct-tcpip": // ssh fowarding
		errMsg := fmt.Sprintf("forbidden channel type: %s", channelType)
		log.Print(errMsg + "\n")

		sshChannel, _, err := sshNewChannel.Accept()
		if err != nil {
			errMsg := fmt.Sprintf("connection failed: because of %s", err.Error())
			log.Print(errMsg + "\n")
			err := sshNewChannel.Reject(ssh.ConnectionFailed, errMsg)
			if err != nil {
				log.Print("reject Failed:", err.Error()+"\n")
				return err
			}
			return nil
		}

		defer sshChannel.Close()

		_, err = sshChannel.Write([]byte(" "))
		if err != nil {
			errMsg := fmt.Sprintf("write data failed: because of %s", err.Error())
			log.Print(errMsg + "\n")
			err := sshNewChannel.Reject(ssh.ConnectionFailed, errMsg)
			if err != nil {
				log.Print("reject Failed:", err.Error()+"\n")
				return err
			}
			return nil
		}

		return nil
	case "session":
		sshChannel, sshRequest, err := sshNewChannel.Accept()
		if err != nil {
			errMsg := fmt.Sprintf("connection failed: because of %s", err.Error())
			log.Print(errMsg + "\n")
			err := sshNewChannel.Reject(ssh.ConnectionFailed, errMsg)
			if err != nil {
				log.Print("reject Failed:", err.Error()+"\n")
				return err
			}
			return nil
		}

		defer sshChannel.Close()

		kernelVersions := []string{Ubuntu, KaliLinux, RaspberryPi, AmazonLinux, CentOS, Debian}

		rand.Seed(time.Now().UnixNano())

		kernelInfo := kernelVersions[rand.Intn(len(kernelVersions))]

		fmt.Fprint(logFile, "OS:"+kernelInfo+"\n")

		for c := range sshRequest {
			if c.Type == "shell" {
				fmt.Fprint(logFile, "RequestTyped:Shell"+"\n\n\n")

				err := handleShell(sshChannel, logFile, userName, kernelInfo)

				if err != nil {
					log.Print("handle shell error:", err.Error()+"\n")
					return err
				}

				return nil

			} else if c.Type == "env" {

			} else if c.Type == "pty-req" {

			} else if c.Type == "exec" {
				fmt.Fprint(logFile, "RequestTyped:Exec"+"\n\n\n")
				err := handleExec(sshChannel, c, logFile, userName, kernelInfo)

				if err != nil {
					log.Print("handle exec error:", err.Error()+"\n")
					return err
				}

				err = sshChannel.Close()
				if err != nil {
					log.Print("channel close failed:", err.Error()+"\n")
					return err
				}
				return nil
			} else {
				log.Print("unknown ssh request type:", c.Type+"\n")
			}
		}
	default:
		errMsg := fmt.Sprintf("unknown channel type: %s", channelType)
		log.Print(errMsg + "\n")
		err := sshNewChannel.Reject(ssh.UnknownChannelType, errMsg)
		if err != nil {
			log.Print("reject failed:", err.Error()+"\n")
			return err
		}
		return errors.New(errMsg)
	}

	errMsg := "unknown channel type"
	return errors.New(errMsg)
}

func handleShell(c ssh.Channel, logFile *os.File, userName string, kernelInfo string) error {
	term := term.NewTerminal(c, "")
	lineLabel := userName + "@" + kernelInfo + ":~$ "

	term.SetPrompt(lineLabel + string(term.Escape.Reset))

	var terminalHeader string

	if kernelInfo == Ubuntu {
		// Ubuntu
		terminalHeader = "Linux ubuntu 2.6.20-16-generic #2 SMP Thu Jun 7 19:00:28 UTC 2007 x86_64\n\nThe programs included with the Ubuntu system are free software;\nthe exact distribution terms for each program are described in the\nindividual files in /usr/share/doc/*/copyright.\n\nUbuntu comes with ABSOLUTELY NO WARRANTY, to the extent permitted by applicable law.\n\nLast login: Mon Aug 13 01:05:46 2007 from 93.184.216.34\n"
	} else if kernelInfo == KaliLinux {
		// KaliLinux
		terminalHeader = "Linux kali 4.14.71-v8 #1 SMP PREEMPT Wed Oct 31 21:41:06 UTC 2018 aarch64\n\nThe programs included with the Kali GNU/Linux system are free software;\nthe exact distribution terms for each program are described in the\nindividual files in /usr/share/doc/*/copyright.\n\nKali GNU/Linux comes with ABSOLUTELY NO WARRANTY, to the extent\npermitted by applicable law.\nLast login: Thu Feb  1 13:51:02 2018 from 93.184.216.34\n"
	} else if kernelInfo == AmazonLinux {
		// AmazonLinux
		terminalHeader = "Last login: Sat Jun  1 09:34:32 2019 from 93.184.216.34\n\n__|  __|_  )\n_|  (     /   Amazon Linux 2 AMI\n___|\\___|___\n\nhttps://aws.amazon.com/amazon-linux-2/\n5 package(s) needed for security, out of 7 available\nRun \"sudo yum update\" to apply all updates.\n"
	} else if kernelInfo == RaspberryPi {
		// RaspberryPi
		terminalHeader = "Linux rasPi 4.9.41-v7+ #1023 SMP Tue Aug 8 16:00:15 BST 2017 armv7l\n\nThe programs included with the Debian GNU/Linux system are free software;\nthe exact distribution terms for each program are described in the\nindividual files in /usr/share/doc/*/copyright.\n\nDebian GNU/Linux comes with ABSOLUTELY NO WARRANTY, to the extent\npermitted by applicable law.\nLast login: Wed Oct 11 18:54:03 2017 from 93.184.216.34\n"
	} else if kernelInfo == Debian {
		// Debian
		terminalHeader = "Linux debian 3.6.5-x86_64 #1 SMP Sun Nov 4 12:40:43 EST 2012 x86_64\n\nThe programs included with the Debian GNU/Linux system are free software;\nthe exact distribution terms for each program are described in the\nindividual files in /usr/share/doc/*/copyright.\n\nDebian GNU/Linux comes with ABSOLUTELY NO WARRANTY, to the extent\npermitted by applicable law.\nLast login: Sat Dec 22 13:38:52 2012 from 93.184.216.34\n"
	} else if kernelInfo == CentOS {
		// CentOS
		terminalHeader = "Last login: Thu Feb  1 13:51:02 2018 from 93.184.216.34\n"
	} else {
		errMsg := "unknown kernel info: " + kernelInfo + "\n"
		log.Print(errMsg)
		return errors.New(errMsg)
	}

	fmt.Fprint(term, terminalHeader)
	fmt.Fprint(logFile, terminalHeader)

	for {
		line, err := term.ReadLine()
		if err == io.EOF {
			log.Print("read eof", "\n")
			return nil
		}
		if err != nil {
			log.Print("read line failed:", err.Error()+"\n")
			return err
		}
		if line == "" {
			fmt.Fprint(term, line)
			fmt.Fprint(logFile, lineLabel+line+"\n")
			continue
		}

		err = emulateCommand([]byte(line), lineLabel, kernelInfo, term, logFile)
		if err != nil {
			log.Print(err.Error() + "\n")
			return err
		}

	}
}

func emulateCommand(v []byte, lineLabel string, kernelInfo string, term *term.Terminal, logFile *os.File) error {
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
	fmt.Println("$", commandName, strings.Join(commandArgs, " "))
	command := commandName + " "

	for _, v := range commandArgs {
		command += v
	}

	fmt.Fprint(logFile, lineLabel+string(v)+"\n")
	msg := command
	if commandName == "uname" {
		if len(commandArgs) > 0 {
			if commandArgs[0] == "-a" {
				if kernelInfo == Ubuntu {
					// Ubuntu
					msg = "Linux ubuntu 4.10.0-35-generic #39~16.04.1-Ubuntu SMP Wed Sep 13 09:02:42 UTC 2017 x86_64 GNU/Linux\n"
				} else if kernelInfo == KaliLinux {
					// KaliLinux
					msg = "Linux kali 4.14.71-v8 #1 SMP PREEMPT Wed Oct 31 21:41:06 UTC 2018 aarch64 GNU/Linux\n"
				} else if kernelInfo == AmazonLinux {
					// AmazonLinux
					msg = "Linux ip-170-31-81-10.ec2.internal 4.10.109-90.92.amzn2.x86_64 #1 SMP Mon Apr 1 23:00:38 UTC 2019 x86_64 x86_64 x86_64 GNU/Linux\n"
				} else if kernelInfo == RaspberryPi {
					// RaspberryPi
					msg = "Linux raspberrypi 3.18.11-v7+ #781 SMP PREEMPT Tue Apr 21 18:07:59 BST 2015 armv7l GNU/Linux\n"
				} else if kernelInfo == Debian {
					// Debian
					msg = "Linux debian 3.2.0-4-amd64 #1 SMP Debian 3.2.65-1+deb7u2 x86_64 GNU/Linux\n"
				} else if kernelInfo == CentOS {
					// CentOS
					msg = "Linux cent 3.10.0-327.28.2.el7.x86_64 #1 SMP Wed Aug 3 11:11:39 UTC 2016 x86_64 x86_64 x86_64 GNU/Linux\n"
				} else {
					errMsg := "unknown kernel info: " + kernelInfo + "\n"
					log.Print(errMsg)
					return errors.New(errMsg)
				}
			}
		} else {
			msg = "Linux\n"
		}
		fmt.Fprint(term, msg)
		fmt.Fprint(logFile, msg)
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
		fmt.Fprint(logFile, msg)
	} else {
		msg = string(v) + "\n"
		fmt.Fprint(term, msg)
		fmt.Fprint(logFile, msg)
	}

	return nil
}

func handleExec(c ssh.Channel, r *ssh.Request, logFile *os.File, userName string, kernelInfo string) error {
	term := term.NewTerminal(c, "")

	lineLabel := userName + "@" + kernelInfo + ":~$ "

	term.SetPrompt(lineLabel + string(term.Escape.Reset))

	err := emulateCommand(r.Payload, lineLabel, kernelInfo, term, logFile)
	if err != nil {
		log.Print(err.Error() + "\n")
		return err
	}
	return nil
}
