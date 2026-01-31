package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"thesis-experiment/internal/queue"
	"time"

	"github.com/joho/godotenv"
)

// seeder berfungsi untuk memasukkan task ke dalam queue
// jumlah task ditentukan lewat flag -n
func main() {
	godotenv.Load()
	count := flag.Int("n", 100, "Images count")
	flag.Parse()

	uploadDir := os.Getenv("STORAGE_PATH_UPLOAD")
	if uploadDir == "" {
		uploadDir = "storage/uploads"
	}

	mq, err := queue.NewRabbitMQService(os.Getenv("RABBITMQ_URL"), os.Getenv("RABBITMQ_QUEUE"))
	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ: %v", err)
	}

	// baca daftar file dari folder upload
	files, err := os.ReadDir(uploadDir)
	if err != nil {
		log.Fatalf("Failed to read upload dir: %v", err)
	}

	// filter hanya file gambar yang valid
	// abaikan folder dan hidden files
	var validFiles []string
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		name := f.Name()
		if len(name) > 4 && name[0] != '.' {
			validFiles = append(validFiles, name)
		}
	}

	if len(validFiles) == 0 {
		log.Printf("WARNING: No images found in %s. Please copy dataset files there.", uploadDir)
		return
	}

	// jika request lebih banyak dari file yang ada
	// maka gunakan semua file yang tersedia
	limit := *count
	if limit > len(validFiles) {
		log.Printf("Requested %d images, but only found %d. Seeding all available.", limit, len(validFiles))
		limit = len(validFiles)
	}

	// kirim setiap file sebagai task ke queue
	for i := 0; i < limit; i++ {
		fileName := validFiles[i]
		taskID := fmt.Sprintf("task-%d", i+1)
		
		err := mq.Publish(context.Background(), taskID, fileName)
		if err != nil {
			log.Printf("Failed to publish %s: %v", fileName, err)
		}
	}

	log.Printf("Seeded %d tasks from real dataset.", limit)
	
	// tunggu sebentar agar pesan terkirim semua
	time.Sleep(1 * time.Second)
	mq.Close()
}
