package main

import (
	"fmt"
	"html/template"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/gorilla/mux"

	"webapp/models"
	"webapp/wp"
	"webapp/wp/api"
)

var homepageTmpl *template.Template
var postsTmpl *template.Template
var tagTmpl *template.Template

func init() {
	var tmplCommon = template.Must(
		template.
			New("Undefined").
			Funcs(template.FuncMap{
				"CurrentYear": func() string {
					return time.Now().Format("2006")
				},
			}).ParseFiles(
			"templates/_layout.tmpl", "templates/_header.tmpl", "templates/_footer.tmpl",
		))

	makeTmpl := func(file string) *template.Template {
		c := template.Must(tmplCommon.Clone())
		return template.Must(c.ParseFiles(file))
	}

	homepageTmpl = makeTmpl("templates/post-index.tmpl")
	postsTmpl = makeTmpl("templates/post-show.tmpl")
	tagTmpl = makeTmpl("templates/tag-show.tmpl")
}

func main() {
	r := mux.NewRouter()

	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		var posts *[]wp.WPPost
		var tags *[]wp.WPTag
		var postErr, tagErr error

		wg := sync.WaitGroup{}

		wg.Add(2)
		go func() { defer wg.Done(); posts, _, postErr = api.Posts().GetAll() }()
		go func() { defer wg.Done(); tags, _, tagErr = api.Tags().GetAll() }()
		wg.Wait()

		if postErr != nil {
			fmt.Fprintf(w, "post error:\n %v", postErr.Error())
		}
		if tagErr != nil {
			fmt.Fprintf(w, "tag error:\n %v", tagErr.Error())
		}
		if postErr != nil || tagErr != nil {
			w.WriteHeader(503)
			return
		}

		tagIdMap := map[int]wp.WPTag{}
		for _, tag := range *tags {
			tagIdMap[tag.Id] = tag
		}

		err := homepageTmpl.ExecuteTemplate(w, "_layout.tmpl", models.PageData{
			Title:   "Posts",
			Request: *r,
			Data:    map[string]any{"posts": posts, "tags": tags, "tagIdMap": tagIdMap},
		})

		if err != nil {
			w.WriteHeader(503)
			fmt.Fprint(w, "There was an error executing the templates.\n", err.Error())
		}
	})

	r.HandleFunc("/posts/{slug}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		slug := vars["slug"]

		posts, _, err := api.Posts().
			SetParam("slug", slug).
			SetParam("per_page", 1).
			Get()

		if err != nil {
			w.WriteHeader(503)
			fmt.Fprintf(w, "post error:\n %v", err.Error())
			return
		}
		if len(*posts) == 0 {
			w.WriteHeader(404)
			fmt.Fprint(w, "Post not found.\n")
			fmt.Fprint(w, "http://wordpress:80/wp-json/wp/v2/posts?slug="+slug)
			return
		}
		p := (*posts)[0]

		tags, _, err := api.Tags().SetParam("include", p.Tags).GetAll()
		if err != nil {
			w.WriteHeader(503)
			fmt.Fprint(w, "There was an error.\n", err.Error())
			return
		}
		tagIdMap := map[int]wp.WPTag{}
		for _, t := range *tags {
			tagIdMap[t.Id] = t
		}

		err = postsTmpl.ExecuteTemplate(w, "_layout.tmpl", models.PageData{
			Title:   p.Title.Rendered,
			Request: *r,
			Data:    map[string]any{"post": p, "tagIdMap": tagIdMap},
		})

		if err != nil {
			w.WriteHeader(503)
			fmt.Fprint(w, "There was an error executing the templates.\n", err.Error())
			return
		}
	})

	r.HandleFunc("/tags/{slug}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		slug := vars["slug"]

		tags, _, err := api.Tags().
			SetParam("slug", slug).
			SetParam("per_page", 1).
			Get()

		if err != nil {
			w.WriteHeader(503)
			fmt.Fprintf(w, "tag error:\n %v", err.Error())
			return
		}
		if len(*tags) == 0 {
			w.WriteHeader(404)
			fmt.Fprint(w, "Post not found.\n")
			fmt.Fprint(w, "http://wordpress:80/wp-json/wp/v2/posts?slug="+slug)
			return
		}
		t := (*tags)[0]

		posts, _, err := api.Posts().SetParam("tags", t.Id).GetAll()
		if err != nil {
			w.WriteHeader(503)
			fmt.Fprint(w, "there was an error.\n", err.Error())
			return
		}

		postTagIds := []int{}
		for _, p := range *posts {
			postTagIds = append(postTagIds, p.Tags...)
		}
		// Dedupe IDs. https://stackoverflow.com/a/76471309/7759523
		slices.Sort(postTagIds) // mutates original slice.
		uniqTagIds := slices.Compact(postTagIds)

		postTags, _, err := api.Tags().SetParam("include", uniqTagIds).GetAll()
		if err != nil {
			w.WriteHeader(503)
			fmt.Fprint(w, "there was an error.\n", err.Error())
			return
		}
		tagIdMap := map[int]wp.WPTag{}
		for _, t := range *postTags {
			tagIdMap[t.Id] = t
		}

		err = tagTmpl.ExecuteTemplate(w, "_layout.tmpl", models.PageData{
			Title:   template.HTML(t.Name),
			Request: *r,
			Data:    map[string]any{"tag": t, "posts": posts, "tagIdMap": tagIdMap},
		})

		if err != nil {
			w.WriteHeader(503)
			fmt.Fprint(w, "There was an error executing the templates.\n", err.Error())
			return
		}
	})

	http.ListenAndServe(":3000", r)
}
