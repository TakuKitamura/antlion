import getpass
import telnetlib

HOST = "localhost"

tn = telnetlib.Telnet(HOST)

tn.write(b"root\n")
tn.write(b"password\n")
tn.write(b"ls\n")
tn.write(b"pwd\n")
tn.write(b"exit\n")
print(tn.read_all().decode('utf-8'))
