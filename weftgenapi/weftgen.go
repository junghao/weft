// weftgen generates http handler wiring with Accept header routing from a TOML file.
// weft.CheckQuery(...) is added based on the Required and Optional query parameters.
// The Content-Type for the response is set based on the Accept header.
//
// Expects config to be a file called weft.toml
// Generates handlers to handlers_auto.go
package main

import (
	"io/ioutil"
	"github.com/naoina/toml"
	"strings"
	"bytes"
	"fmt"
	"os"
	"log"
)

type api struct {
	Endpoint []endpoint
}

type endpoint struct {
	Uri    string
	Get    []request // there can be multiple GET requests routed by the Accept header.
	Put    *request
	Delete *request
}

type request struct {
	Function  string // name of the weft.RequestHandler func that will handle the request.
	Accept    string // GET requests are routed with exact Accept matching.
	Default   bool // for GET requests to and endpoint one request may be the default for any unmatched Accept headers.
	Required []string  // required query parameters.
	Optional []string // optional query parameters.
}

func main() {
	a := api{}

	if err := a.read("weft.toml"); err != nil {
		log.Fatal(err)
	}

	if err := a.writeHandlers("handlers_auto.go"); err != nil {
		log.Fatal(err)
	}
}

func handlerName(f string) string {
	if strings.HasSuffix(f, "/") {
		f = f + "s"
	}

	return strings.Replace(f, "/", "", -1) + "Handler"
}

// check writes a checkQuery func to b
func (a request) checkQuery(b *bytes.Buffer) {
	var r, o string

	if len(a.Required) > 0 {
		r = `"` + strings.Join(a.Required, `", "`) + `"`

	}

	if len(a.Optional) > 0 {
		o = `"` + strings.Join(a.Optional, `", "`) + `"`

	}

	b.WriteString(fmt.Sprintf("if res := weft.CheckQuery(r, []string{%s}, []string{%s}); !res.Ok {\n", r, o))
	b.WriteString("return res\n")
	b.WriteString("}\n")
}

func (a *api) read(filename string) error {
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}

	return toml.Unmarshal(b, &a)
}

func (a *api) writeHandlers(filename string) error {
	var b bytes.Buffer

	b.WriteString(`package main` + "\n")
	b.WriteString("\n")
	b.WriteString("// This file is auto generated - do not edit.\n")
	b.WriteString("// It was created with weftgenapi from github.com/GeoNet/weft/weftgenapi\n")
	b.WriteString("\n")
	b.WriteString(`import (` + "\n")
	b.WriteString(`"bytes"` + "\n")
	b.WriteString(`"github.com/GeoNet/weft"` + "\n")
	b.WriteString(`"net/http"` + "\n")
	b.WriteString(`)` + "\n")
	b.WriteString("\n")
	b.WriteString("var mux = http.NewServeMux()\n")
	b.WriteString("\n")

	// the init() func - add routes the mux

	b.WriteString("\n")
	b.WriteString("func init() {\n")

	for _, e := range a.Endpoint {
		b.WriteString(fmt.Sprintf("mux.HandleFunc(\"%s\", weft.MakeHandlerAPI(%s))\n", e.Uri, handlerName(e.Uri)))
	}

	b.WriteString("}\n")
	b.WriteString("\n")

	// create the handler func for each endpoint with a method switch and an accept
	// switch for GET.

	for _, e := range a.Endpoint {
		b.WriteString(fmt.Sprintf("func %s(r *http.Request, h http.Header, b *bytes.Buffer) *weft.Result {\n", handlerName(e.Uri)))
		b.WriteString("switch r.Method {\n")

		if len(e.Get) > 0 {
			b.WriteString(`case "GET":` + "\n")
			b.WriteString(`switch r.Header.Get("Accept") {` + "\n")

			var d request
			var hasDefault bool

			for _, r := range e.Get {
				if r.Default {
					if hasDefault {
						return fmt.Errorf("found multiple defaults for %s GET", e.Uri)
					}
					hasDefault = true
					d = r
				}

				b.WriteString(fmt.Sprintf("case \"%s\":\n", r.Accept))
				r.checkQuery(&b)
				b.WriteString(fmt.Sprintf("h.Set(\"Content-Type\", \"%s\")\n", r.Accept))
				b.WriteString(fmt.Sprintf("return %s(r, h, b)\n", r.Function))
			}

			b.WriteString("default:\n")
			if hasDefault {
				d.checkQuery(&b)
				b.WriteString(fmt.Sprintf("h.Set(\"Content-Type\", \"%s\")\n", d.Accept))
				b.WriteString(fmt.Sprintf("return %s(r, h, b)\n", d.Function))
			} else {
				b.WriteString("return &weft.NotAcceptable\n")
			}

			b.WriteString("}\n")
		}

		if e.Put != nil {
			b.WriteString(`case "PUT":` + "\n")
			e.Put.checkQuery(&b)
			b.WriteString(fmt.Sprintf("return %s(r, h, b)\n", e.Put.Function))
		}

		if e.Delete != nil {
			b.WriteString(`case "DELETE":` + "\n")
			e.Delete.checkQuery(&b)
			b.WriteString(fmt.Sprintf("return %s(r, h, b)\n", e.Delete.Function))
		}

		b.WriteString("default:\n")
		b.WriteString("return &weft.MethodNotAllowed\n")
		b.WriteString("}\n")
		b.WriteString("}\n")
		b.WriteString("\n")
	}

	f, err := os.Create(filename)
	if err != nil {
		return err
	}

	defer f.Close()

	_, err = b.WriteTo(f)
	if err != nil {
		return err
	}

	return f.Sync()
}

