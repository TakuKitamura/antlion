package main

import (
	"antlion/app/proto"
	"sync"
)

func main() {
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		proto.StartTelnetServer()
		wg.Done()
	}()
	go func() {
		proto.StartSshSerer()
		wg.Done()
	}()
	wg.Wait()
}
