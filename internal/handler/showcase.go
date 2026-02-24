package handler

import (
	"net/http"
	"os"
)

func Showcase() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := os.ReadFile("web/showcase.html")
		if err != nil {
			http.Error(w, "showcase page not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	}
}
