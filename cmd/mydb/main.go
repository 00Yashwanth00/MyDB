package main

import (
	"fmt"
	"log"

	"mydb/internal/storage"
)

func main() {
	err := storage.SaveData1(
		"test.db",
		[]byte("Hello database!"),
	)

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Data saved successfully")
}
