runRaspi:
	docker build -t antlion . && docker run -p 2222:2222 -p 5555:5555 -it --rm --name running-antlion antlion
runMac:
	docker build -t antlion --build-arg IMAGE=golang:latest . && docker run -p 2222:2222 -p 5555:5555 -it --rm --name running-antlion antlion
bash:
	docker exec -it running-antlion sh