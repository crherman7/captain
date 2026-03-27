package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq"
)

func cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgresql://hello:hello@localhost:5432/hello?sslmode=disable"
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("connecting to database: %v", err)
	}
	defer db.Close()

	for i := 0; i < 30; i++ {
		if err := db.Ping(); err == nil {
			break
		}
		log.Printf("waiting for database...")
		time.Sleep(time.Second)
	}

	if err := db.Ping(); err != nil {
		log.Fatalf("database not reachable: %v", err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS greetings (
		id SERIAL PRIMARY KEY,
		message TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT NOW()
	)`)
	if err != nil {
		log.Fatalf("creating table: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM greetings").Scan(&count); err != nil {
		log.Fatalf("counting greetings: %v", err)
	}
	if count == 0 {
		if _, err := db.Exec("INSERT INTO greetings (message) VALUES ('Hello from Captain!')"); err != nil {
			log.Fatalf("seeding greeting: %v", err)
		}
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", cors(func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(); err != nil {
			http.Error(w, `{"status":"unhealthy"}`, http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}` + "\n"))
	}))

	mux.HandleFunc("/api/hello", cors(func(w http.ResponseWriter, r *http.Request) {
		var msg string
		err := db.QueryRow("SELECT message FROM greetings ORDER BY id DESC LIMIT 1").Scan(&msg)
		if err != nil {
			http.Error(w, `{"error":"no greetings found"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": msg})
	}))

	mux.HandleFunc("/api/greetings", cors(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			rows, err := db.Query("SELECT id, message, created_at FROM greetings ORDER BY id DESC LIMIT 20")
			if err != nil {
				http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			type greeting struct {
				ID        int    `json:"id"`
				Message   string `json:"message"`
				CreatedAt string `json:"createdAt"`
			}
			var greetings []greeting
			for rows.Next() {
				var g greeting
				var t time.Time
				if err := rows.Scan(&g.ID, &g.Message, &t); err != nil {
					continue
				}
				g.CreatedAt = t.Format(time.RFC3339)
				greetings = append(greetings, g)
			}
			if greetings == nil {
				greetings = []greeting{}
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(greetings)

		case http.MethodPost:
			var body struct {
				Message string `json:"message"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Message == "" {
				http.Error(w, `{"error":"message is required"}`, http.StatusBadRequest)
				return
			}
			var id int
			err := db.QueryRow("INSERT INTO greetings (message) VALUES ($1) RETURNING id", body.Message).Scan(&id)
			if err != nil {
				http.Error(w, `{"error":"insert failed"}`, http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]int{"id": id})

		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	}))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("API listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
