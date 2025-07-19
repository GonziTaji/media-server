package main
import (
	"net/http"
	"fmt"
	"log"
	"strings"
	"text/scanner"
	"os"
	"html/template"
)

const PORT = ":8080"
const MEDIAROOT = "/home/yogusita/"

func parseUrlPath(path string) []string {
	var s scanner.Scanner
	s.Init(strings.NewReader(path))

	var parts []string

	for {
		tok := s.Scan()

		if tok == scanner.EOF {
			break
		}

		token := s.TokenText()

		if token != "/" {
			parts = append(parts, token)
		}
	}

	return parts
}

type MainHandler struct {}

func (MainHandler) ServeHTTP (w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Main handler hit with path: %s\n", r.URL.Path)
}

type MediaHandler struct {}

type PageHomeData struct {
	Title string	
	Heading string
	Ruta string
	Files []os.DirEntry
	// Files []string
}

func(MediaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Media handler hit with path: %s\n", r.URL.Path)

	files, err := os.ReadDir(MEDIAROOT)

	if err != nil {
		log.Fatal(err)
	}

	var dirEntries []os.DirEntry
	// var dirEntries []string

	for _, file := range files  {
		var decorator string
		if (file.IsDir()) {
			decorator = "d"
		} else {
			decorator = "f"
		}

		fmt.Println(decorator)
		dirEntries = append(dirEntries, file)
	}

	tmpl := template.Must(template.ParseFiles("templates/base.html", "templates/home.html"))
	data := PageHomeData {
		Title: "Yogusita Media Server",
		Heading: "Bienvenid@ a Yogusita Media Server",
		Ruta: MEDIAROOT,
		Files: dirEntries,
	}

	if err := tmpl.ExecuteTemplate(w, "base.html", data); err != nil {
	    log.Fatalln(err)
	}
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/favicon.ico", func (w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "a")
	})
	mux.Handle("/", MainHandler{})
	mux.Handle("/media/", MediaHandler{})

	fmt.Printf("Listening on port %s\n", PORT)

	log.Panic(http.ListenAndServe(PORT, mux))
}
