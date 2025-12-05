package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/alicebob/miniredis/v2"
)

func main() {
	s := miniredis.NewMiniRedis()
	if err := s.StartAddr("127.0.0.1:6379"); err != nil {
		log.Fatalf("Failed to start miniredis: %v", err)
	}
	defer s.Close()

	log.Printf("MiniRedis server started on %s", s.Addr())

	// Wait for interrupt signal to gracefully shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down MiniRedis...")
}
