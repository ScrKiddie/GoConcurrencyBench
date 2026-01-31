package queue

import (
	"context"
	"encoding/json"

	"log"
	"sync"
	"thesis-experiment/internal/entity"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type RabbitMQService struct {
	conn      *amqp.Connection
	channel   *amqp.Channel
	queueName string
	url       string
	mu        sync.Mutex
}

// inisialisasi koneksi ke rabbitmq
func NewRabbitMQService(url, queueName string) (*RabbitMQService, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, err
	}
	ch, err := conn.Channel()
	if err != nil {
		return nil, err
	}

	// deklarasi queue dengan tipe quorum untuk durabilitas
	_, err = ch.QueueDeclare(queueName, true, false, false, false, amqp.Table{
		"x-queue-type": "quorum",
	})
	if err != nil {
		return nil, err
	}

	return &RabbitMQService{conn: conn, channel: ch, queueName: queueName, url: url}, nil
}

func (r *RabbitMQService) Close() {
	if r.channel != nil { r.channel.Close() }
	if r.conn != nil { r.conn.Close() }
}

// kirim task ke queue
func (r *RabbitMQService) Publish(ctx context.Context, id, fileName string) error {
	body, _ := json.Marshal(entity.TaskPayload{ID: id, FileName: fileName})
	return r.channel.PublishWithContext(ctx, "", r.queueName, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
	})
}

// ambil task dari queue dalam bentuk batch
// fungsi ini menunggu sampai jumlah task sesuai batchsize
// atau sampai timeout tercapai
func (r *RabbitMQService) ConsumeBatch(ctx context.Context, batchSize int, timeout time.Duration, handler func([]entity.TaskPayload) error) error {
	msgs, err := r.channel.Consume(r.queueName, "", false, false, false, false, nil)
	if err != nil {
		return err
	}

	var batch []entity.TaskPayload
	var deliveries []amqp.Delivery
	timer := time.NewTimer(timeout)

	log.Println("Waiting for tasks...")

	for {
		select {
		case msg, ok := <-msgs:
			if !ok {
				return nil
			}
			var p entity.TaskPayload
			if err := json.Unmarshal(msg.Body, &p); err == nil {
				batch = append(batch, p)
				deliveries = append(deliveries, msg)
			}
			
			// jika sudah cukup maka proses batch
			if len(batch) >= batchSize {
				timer.Stop()
				if err := handler(batch); err != nil {
					return err
				}
				// kirim ack untuk semua pesan yang sudah diproses
				for _, d := range deliveries { d.Ack(false) }
				return nil
			}
		case <-timer.C:
			// timeout tercapai
			// proses batch yang sudah terkumpul meski belum penuh
			if len(batch) > 0 {
				if err := handler(batch); err != nil {
					return err
				}
				for _, d := range deliveries { d.Ack(false) }
			}
			return nil
		}
	}
}
