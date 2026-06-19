package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

type URLMapping struct {
	OriginURL string    `firestore:"origin_url"`
	CreatedAt time.Time `firestore:"created_at"`

	AccessCount int `firestore:"access_count"`
}

var client *firestore.Client

func main() {
	ctx := context.Background()
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		log.Fatal("GOOGLE_CLOUD_PROJECT 環境変数を設定してください")
	}

	var err error
	client, err = firestore.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("Firestoreの初期化に失敗: %v", err)
	}
	defer client.Close()

	http.HandleFunc("/", Hindex)
	http.HandleFunc("/shorten", Hshorten)
	http.HandleFunc("/r/", Hredirect)
	http.HandleFunc("/countpage", Hcountpage)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("サーバーをポート %s で起動中...", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

func Hindex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, "index.html")
}

func Hshorten(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	originURL := r.FormValue("url")
	if originURL == "" {
		http.Error(w, "URLが空です", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	newDocRef := client.Collection("urls").NewDoc()
	id := newDocRef.ID

	_, err := newDocRef.Set(ctx, URLMapping{
		OriginURL:   originURL,
		CreatedAt:   time.Now(),
		AccessCount: 0,
	})
	if err != nil {
		http.Error(w, "データの保存に失敗しました", http.StatusInternalServerError)
		return
	}

	scheme := "http://"
	if r.TLS != nil {
		scheme = "https://"
	}
	shortenedURL := scheme + r.Host + "/r/" + id
	w.Write([]byte("短縮URLが生成されました: " + shortenedURL))
}

func Hredirect(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[3:]
	if id == "" {
		http.Error(w, "IDが不正です", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	doc, err := client.Collection("urls").Doc(id).Get(ctx)
	if err != nil {
		if err == iterator.Done {
			http.Error(w, "URLが見つかりません", http.StatusNotFound)
		} else {
			http.Error(w, "エラーが発生しました", http.StatusInternalServerError)
		}
		return
	}

	var mapping URLMapping
	if err := doc.DataTo(&mapping); err != nil {
		http.Error(w, "データのパースに失敗しました", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=43200")

	http.Redirect(w, r, mapping.OriginURL, http.StatusMovedPermanently)
}

func Hcountpage(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	id = strings.TrimPrefix(id, "r/")
	id = strings.TrimSpace(id)

	if id == "" {
		http.Error(w, "IDが必要です", http.StatusBadRequest)
		return
	}

	ctx := context.Background()

	_, err := client.Collection("urls").Doc(id).Update(ctx, []firestore.Update{
		{Path: "access_count", Value: firestore.FieldValue.Increment(1)},
	})
	if err != nil {
		http.Error(w, "カウント更新失敗。IDが存在しない可能性があります。", http.StatusInternalServerError)
		return
	}
	w.Write([]byte("Counted"))
}
