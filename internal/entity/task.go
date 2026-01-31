package entity

// struktur data untuk task yang dikirim lewat queue
// berisi id unik dan nama file gambar yang akan diproses
type TaskPayload struct {
	ID       string `json:"id"`
	FileName string `json:"file_name"`
}
