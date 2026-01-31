package compression

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"thesis-experiment/internal/entity"
	"time"

	"thesis-experiment/vips"
	"github.com/shirou/gopsutil/v3/process"
)

type Config struct {
	UploadDir     string
	CompressedDir string
	ResultPath    string
	IsConcurrent  bool
	UseWorkerPool bool
	NumWorkers    int
}

type Service struct {
	Config Config
}

func NewService(cfg Config) *Service {
	return &Service{Config: cfg}
}

// fungsi utama untuk menjalankan eksperimen
// di sini saya mengukur waktu eksekusi cpu dan ram
func (s *Service) RunExperiment(ctx context.Context, tasks []entity.TaskPayload) error {
	// bersihkan memori sebelum mulai agar pengukuran tidak bias
	runtime.GC()
	debug.FreeOSMemory()

	log.Printf("Starting Batch: %d images | Mode: %s", len(tasks), s.getModeName())

	startTime := time.Now()
	proc, _ := process.NewProcess(int32(os.Getpid()))
	cpuBefore, _ := proc.Times()
	
	// jalankan goroutine terpisah untuk memantau pemakaian ram
	// ini berjalan paralel dengan proses utama
	doneMon := make(chan struct{})
	ramChan := make(chan uint64, 1)
	go s.monitorRAM(proc, doneMon, ramChan)

	// pilih model pemrosesan berdasarkan konfigurasi
	if s.Config.IsConcurrent {
		if s.Config.UseWorkerPool {
			s.runWorkerPool(tasks)
		} else {
			s.runNaive(tasks)
		}
	} else {
		s.runSequential(tasks)
	}

	duration := time.Since(startTime)
	close(doneMon)
	
	// ambil nilai peak ram dari goroutine monitor
	peakRAM := <-ramChan
	cpuAfter, _ := proc.Times()
	cpuUsed := (cpuAfter.User + cpuAfter.System) - (cpuBefore.User + cpuBefore.System)
	cpuPercent := 0.0
	if duration.Seconds() > 0 {
		cpuPercent = (cpuUsed / duration.Seconds()) * 100 
	}

	return s.saveToCSV(len(tasks), duration, cpuPercent, peakRAM)
}


// model sekuensial
// proses satu per satu secara berurutan
func (s *Service) runSequential(tasks []entity.TaskPayload) {
	for _, t := range tasks {
		s.processImage(t)
	}
}

// model konkuren naif
// setiap task langsung dibuatkan goroutine baru
// tidak ada batasan jumlah goroutine yang berjalan
func (s *Service) runNaive(tasks []entity.TaskPayload) {
	var wg sync.WaitGroup
	for _, t := range tasks {
		wg.Add(1)
		go func(task entity.TaskPayload) {
			defer wg.Done()
			s.processImage(task)
		}(t)
	}
	wg.Wait()
}

// model worker pool
// jumlah goroutine dibatasi sesuai numworkers
// task didistribusikan lewat channel
func (s *Service) runWorkerPool(tasks []entity.TaskPayload) {
	jobs := make(chan entity.TaskPayload, len(tasks))
	var wg sync.WaitGroup

	// buat worker sejumlah yang dikonfigurasi
	for i := 0; i < s.Config.NumWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// setiap worker mengambil task dari channel sampai habis
			for t := range jobs {
				s.processImage(t)
			}
		}()
	}

	// kirim semua task ke channel
	for _, t := range tasks { jobs <- t }
	close(jobs)
	wg.Wait()
}

// proses kompresi gambar menggunakan libvips
func (s *Service) processImage(t entity.TaskPayload) {
	src := filepath.Join(s.Config.UploadDir, t.FileName)
	dst := filepath.Join(s.Config.CompressedDir, fmt.Sprintf("%s.webp", t.ID))

	// gunakan mode sequential unbuffered untuk hemat memori
	importParams := &vips.LoadOptions{
		Access: vips.AccessSequentialUnbuffered,
	}
	
	img, err := vips.NewImageFromFile(src, importParams)
	if err != nil {
		log.Printf("Error open %s: %v", t.FileName, err)
		return
	}
	defer img.Close()

	// kualitas webp diset 75 sesuai metodologi
	ep := &vips.WebpsaveOptions{
		Q: 75,
	}
	
	if err := img.Webpsave(dst, ep); err != nil {
		log.Printf("Error save %s: %v", dst, err)
	}
}


// monitor pemakaian ram secara berkala
// mencatat nilai tertinggi selama eksperimen berjalan
func (s *Service) monitorRAM(p *process.Process, done <-chan struct{}, out chan<- uint64) {
	var peak uint64
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			out <- peak
			return
		case <-ticker.C:
			info, _ := p.MemoryInfo()
			if info != nil && info.RSS > peak {
				peak = info.RSS
			}
		}
	}
}

// simpan hasil pengukuran ke file csv
func (s *Service) saveToCSV(count int, dur time.Duration, cpu float64, ram uint64) error {
	fileExists := false
	if _, err := os.Stat(s.Config.ResultPath); err == nil {
		fileExists = true
	}

	f, err := os.OpenFile(s.Config.ResultPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	// tulis header hanya jika file baru dibuat
	if !fileExists {
		w.Write([]string{"Timestamp", "Scenario", "Total_Images", "Duration_Sec", "CPU_Avg_Percent", "Peak_RAM_MB", "Num_Workers"})
	}

	ramMB := float64(ram) / 1024 / 1024
	w.Write([]string{
		time.Now().Format("2006-01-02 15:04:05"),
		s.getModeName(),
		fmt.Sprintf("%d", count),
		fmt.Sprintf("%.4f", dur.Seconds()),
		fmt.Sprintf("%.2f", cpu),
		fmt.Sprintf("%.2f", ramMB),
		fmt.Sprintf("%d", s.Config.NumWorkers),
	})
	
	return nil
}

// tentukan nama skenario berdasarkan konfigurasi
func (s *Service) getModeName() string {
	if !s.Config.IsConcurrent { return "Sequential" }
	if s.Config.UseWorkerPool { return fmt.Sprintf("WorkerPool_%d", s.Config.NumWorkers) }
	return "Naive_Concurrent"
}
