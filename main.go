package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
)

const PORT = ":8080"
const MEDIA_ROOT = "/home/yogusita/"

type PageHomeData struct {
	Title         string
	Heading       string
	CurrentFsPath string
	CurrentUrl    string
	ParentUrl     string
	Files         []os.DirEntry
}

type MainHandler struct{}
type MediaHandler struct{}

func (MainHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Main handler hit with path: %s\n", r.URL.Path)
}

func (h MediaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

		if errIsNotExist {
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
	if fileInfo.IsDir() {
		files, err := os.ReadDir(fsPath)

		if err != nil {
			handleOsError(err)
			return
		}

		tmpl := template.Must(template.ParseFiles("templates/base.tmpl", "templates/directory.tmpl"))
		data := PageHomeData{
			Title:         "Yogusita Media Server",
			Heading:       "Bienvenid@ a Yogusita Media Server",
			CurrentFsPath: fsPath,
			CurrentUrl:    r.URL.Path,
			ParentUrl:     parentDirPath,
			Files:         files,
		}

		if err := tmpl.ExecuteTemplate(w, "base.tmpl", data); err != nil {
			log.Fatalln(err)
		}

		return
	}

	http.ServeFile(w, r, fsPath)
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "a")
	})

	mux.Handle("/", MainHandler{})
	mux.Handle("/media/", MediaHandler{})
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	fmt.Printf("Listening on port %s\n", PORT)

	log.Panic(http.ListenAndServe(PORT, mux))
}
