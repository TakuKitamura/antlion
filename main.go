package main

import (
	"antlion/proto"
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
