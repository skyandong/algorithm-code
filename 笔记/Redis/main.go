package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/redis/go-redis/v9"
)

var client *redis.Client

func main() {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "123456", // no password set
		DB:       0,        // use default DB
	})
	defer rdb.Close()

	pong, err := rdb.Ping(context.Background()).Result()
	if err != nil {
		log.Fatalf("ping: %s,err: %v", pong, err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	<-sigChan

	fmt.Println("quit")
}
