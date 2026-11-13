// Command messenger-api serves the chat API.
package main

import (
	"flag"
	"log"

	"example.local/messenger-api/internal/chat"
	"example.local/messenger-api/internal/storage"
)

func main() {
	config := flag.String("config", "config/local.yaml", "config file")
	flag.Parse()
	db, err := storage.Open(*config)
	if err != nil {
		log.Fatal(err)
	}
	svc := chat.NewService(db)
	log.Fatal(svc.Serve(":8080"))
}
