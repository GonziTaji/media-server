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
const MEDIA_ROOT = "/home/yogusita/"

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
	CurrentFsPath string
	CurrentUrl string
	ParentUrl string
	Files []os.DirEntry
}

func(h MediaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Media handler hit with path: %s\n", r.URL.Path)

	// necesito que si es /media/mi/carpeta
	// localpath es MEDIA_ROOT/mi/carpeta
	fsPath := strings.Replace(r.URL.Path, "/media/", MEDIA_ROOT, 1)
	fmt.Printf("> fsPath : %s\n", fsPath)

	var parentDirPath string

	if !strings.HasSuffix(fsPath, MEDIA_ROOT) {
		// Url no termina en MEDIA_ROOT
		s := strings.Split(r.URL.Path, "/")
		// Quitamos media-query y el ultimo elemento, y unimos para crear el path
		parentDirPath = strings.Join(s[1:len(s)-1], "/")
	}

	fmt.Printf("> parentDirPath : %s\n", parentDirPath)

	handleOsError := func(err error) {
		fmt.Printf("> Error reading path: %s\n", fsPath)
		fmt.Printf("> Error:%s\n", err)
		errIsNotExist := os.IsNotExist(err)

		if (errIsNotExist) {
			// TODO: ir a pagina 404
			fmt.Fprintln(w, "No hay nada aqui")
			return
		}

		// TODO: ir a pagina error
		log.Fatal(err)
	}

	fileInfo, err := os.Lstat(fsPath)

	if err != nil {
		handleOsError(err)
		return
	}
		

	// TODO: handle symlinks?
	if (fileInfo.IsDir()) {
		files, err := os.ReadDir(fsPath)

		if err != nil {
			handleOsError(err)
			return
		}

		tmpl := template.Must(template.ParseFiles("templates/base.html", "templates/directory.html"))
		data := PageHomeData {
			Title: "Yogusita Media Server",
			Heading: "Bienvenid@ a Yogusita Media Server",
			CurrentFsPath: fsPath,
			CurrentUrl: r.URL.Path,
			ParentUrl: parentDirPath, 
			Files: files,
		}

		if err := tmpl.ExecuteTemplate(w, "base.html", data); err != nil {
		    log.Fatalln(err)
		}

		return
	}

	http.ServeFile(w, r, fsPath)
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
