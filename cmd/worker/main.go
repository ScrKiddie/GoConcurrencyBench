package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"thesis-experiment/internal/compression"
	"thesis-experiment/internal/entity"
	"thesis-experiment/internal/queue"
	"time"

	"thesis-experiment/vips"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()
	
	// inisialisasi libvips dengan konfigurasi khusus
	// concurrency level diset 1 agar fokus ke model konkuren go
	// bukan konkurensi internal libvips
	vips.Startup(&vips.Config{
		ConcurrencyLevel: 1,
		MaxCacheFiles:    0,
		MaxCacheMem:      0,
		MaxCacheSize:     0,
		ReportLeaks:      false,
		CacheTrace:       false,
		VectorEnabled:    true,
	})
	defer vips.Shutdown()

	// baca batch size dari environment variable
	// batch size menentukan berapa gambar yang diproses dalam satu run
	batchSizeEnv := os.Getenv("BATCH_SIZE")
	batchSize, err := strconv.Atoi(batchSizeEnv)
	if err != nil || batchSize <= 0 {
		batchSize = 1 
		log.Println("Warning: BATCH_SIZE not set, defaulting to 1")
	}

	// baca konfigurasi skenario dari environment
	isConc, _ := strconv.ParseBool(os.Getenv("IS_CONCURRENT"))
	usePool, _ := strconv.ParseBool(os.Getenv("USE_WORKER_POOL"))
	workers, _ := strconv.Atoi(os.Getenv("NUM_WORKERS"))

	cfg := compression.Config{
		UploadDir:     os.Getenv("STORAGE_PATH_UPLOAD"),
		CompressedDir: os.Getenv("STORAGE_PATH_COMPRESSED"),
		ResultPath:    os.Getenv("RESULT_FILE_PATH"),
		IsConcurrent:  isConc,
		UseWorkerPool: usePool,
		NumWorkers:    workers,
	}

	svc := compression.NewService(cfg)
	mq, err := queue.NewRabbitMQService(os.Getenv("RABBITMQ_URL"), os.Getenv("RABBITMQ_QUEUE"))
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Worker Ready. Waiting for exactly %d tasks...", batchSize)
	
	// consume batch dari queue lalu jalankan eksperimen
	// timeout 1 menit untuk memastikan semua task masuk
	err = mq.ConsumeBatch(context.Background(), batchSize, 1*time.Minute, func(tasks []entity.TaskPayload) error {
		return svc.RunExperiment(context.Background(), tasks)
	})
	if err != nil {
		log.Fatal(err)
	}
	
	log.Println("Batch finished. Exiting to allow clean restart.")
}
