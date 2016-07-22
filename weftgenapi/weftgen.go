// weftgen generates http handler wiring with Accept header routing from a TOML file.
// weft.CheckQuery(...) is added based on the Required and Optional query parameters.
// The Content-Type for the response is set based on the Accept header.
//
// HTML docs are also generated (and a handler to serve them). They are available at http://.../api-docs
//
// Expects config to be a file called weft.toml
// Generates handlers to handlers_auto.go
package main

import (
	"bytes"
	"fmt"
	"github.com/naoina/toml"
	"io/ioutil"
	"log"
	"os"
	"sort"
	"strings"
)

type api struct {
	Production bool   // set true to hide the banner in web pages.
	APIHost    string // the public host name for the service e.g., api.geonet.org.nz
	Title      string // title for the api
	Discussion string // any extended discussion for the api.  Can include HTML and requires surround <p> tags.
	Repo       string
	Endpoint   Endpoint
	Query      map[string]parameter // use the map to group query parameter docs.
	Response   map[string]parameter // use the map to group query parameter docs.
}

type parameter struct {
	Id          string // defaults to the map[string] if zero.
	Description string // a description of the parameter.  Can include HTML, does not need surrounding tags.
	Type        string // the type of the parameter e.g., int32
	// TODO include a list of possible values?  Should this just be a slice of strings?
}

type endpoint struct {
	Uri         string
	Request     Request // allow multiple GET requests routed by Accept.  Only 1 PUT or DELETE.
	Title       string  // the title for the endpoint.  Does not need surrounding tags.
	Description string  // a short description for the endpoint.  Can include HTML, does not need surrounding tags.
	Discussion  string  // any extended discussion for the endpoint.  Can include HTML and requires surround <p> tags.
}

type request struct {
	Method      string   // e.g., GET, PUT, DELETE etc
	Function    string   // name of the weft.RequestHandler func that will handle the request.
	Accept      string   // GET requests are routed with exact Accept matching.
	Default     bool     // for GET requests to and endpoint one request may be the default for any unmatched Accept headers.
	Parameter   string   // a single URI query parameter.  If defined then there should not be Query parameters as well.  Should match an entry in api.Parameter
	Required    []string // required query parameters.  Should match an entry in api.Parameter
	Optional    []string // optional query parameters.  Should match an entry in api.Parameter
	Response    []string // response parameters.  Should match an entry in api.Response
	Group       string   // should match the string in api.Parameter[string]
	Description string   // a short description for the request.  Can include HTML, does not need surrounding tags.
	Discussion  string   // any extended discussion for request or response.  Can include HTML and requires surround <p> tags.

	// the following members do not need to be added to the TOML.  They are for use in HTML templates.
	R   Parameter // query parameters added based on Required and api.Parameter.
	O   Parameter // query parameters added based on Optional and api.Parameter.
	Res Parameter // response parameters added based on Response and api.Parameter.
	P   parameter // URI parameter added based on Parameter and api.Parameter.
	Uri string
}

type Request []request

func (a Request) Len() int {
	return len(a)
}
func (a Request) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}
func (a Request) Less(i, j int) bool {
	return a[i].Method < a[j].Method
}

type Endpoint []endpoint

func (a Endpoint) Len() int {
	return len(a)
}
func (a Endpoint) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}
func (a Endpoint) Less(i, j int) bool {
	return a[i].Title < a[j].Title
}

type Parameter []parameter

func (a Parameter) Len() int {
	return len(a)
}
func (a Parameter) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}
func (a Parameter) Less(i, j int) bool {
	return a[i].Id < a[j].Id
}

func main() {
	a := api{}

	if err := a.read("weft.toml"); err != nil {
		log.Fatal(err)
	}

	if err := a.writeHandlers("handlers_auto.go"); err != nil {
		log.Fatal(err)
	}

	if err := a.writeDocs("assets/api-docs/index.html"); err != nil {
		log.Fatal(err)
	}
}

func (r Request) filter(method string) Request {
	var res Request

	for _, v := range r {
		if v.Method == method {
			res = append(res, v)
		}
	}

	return res
}

func handlerName(f string) string {
	if strings.HasSuffix(f, "/") {
		f = f + "s"
	}

	f = strings.Replace(f, ".", "_", -1)
	f = strings.Replace(f, "-", "_", -1)

	return strings.Replace(f, "/", "", -1) + "Handler"
}

// check writes a checkQuery func to b
func (a request) checkQuery(b *bytes.Buffer) {
	b.WriteString(fmt.Sprintf("if res := weft.CheckQuery(r, []string{%s}, []string{%s}); !res.Ok {\n",
		a.R.checkString(), a.O.checkString()))
	b.WriteString("return res\n")
	b.WriteString("}\n")
}

func (a Parameter) checkString() string {
	var b []string

	for _, v := range a {
		b = append(b, v.Id)
	}

	s := strings.Join(b, `", "`)

	if s != "" {
		s = `"` + s + `"`
	}

	return s
}

func (a *api) read(filename string) error {
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}

	err = toml.Unmarshal(b, &a)
	if err != nil {
		return err
	}

	for k, v := range a.Query {
		if v.Id == "" {
			v.Id = k
			a.Query[k] = v
		}
	}

	for k, v := range a.Response {
		if v.Id == "" {
			v.Id = k
			a.Query[k] = v
		}
	}

	// add the R and O []parameter to each request and sort everything.
	sort.Sort(a.Endpoint)

	for i := range a.Endpoint {
		sort.Sort(a.Endpoint[i].Request)

		for j := range a.Endpoint[i].Request {
			a.Endpoint[i].Request[j].Uri = a.Endpoint[i].Uri

			if a.Endpoint[i].Request[j].Parameter != "" {
				p, ok := a.Query[a.Endpoint[i].Request[j].Parameter]
				if !ok {
					return fmt.Errorf("found no parameter uri parameter for %s %s %s",
						a.Endpoint[i].Title,
						a.Endpoint[i].Request[j].Method,
						a.Endpoint[i].Request[j].Parameter)
				}
				a.Endpoint[i].Request[j].P = p
			}

			for _, s := range a.Endpoint[i].Request[j].Required {
				p, ok := a.Query[s]
				if !ok {
					return fmt.Errorf("found no parameter for %s %s %s",
						a.Endpoint[i].Title,
						a.Endpoint[i].Request[j].Method,
						s)
				}
				a.Endpoint[i].Request[j].R = append(a.Endpoint[i].Request[j].R, p)
			}
			sort.Sort(a.Endpoint[i].Request[j].R)

			for _, s := range a.Endpoint[i].Request[j].Optional {
				p, ok := a.Query[s]
				if !ok {
					return fmt.Errorf("found no parameter for %s %s %s",
						a.Endpoint[i].Title,
						a.Endpoint[i].Request[j].Method,
						s)
				}
				a.Endpoint[i].Request[j].O = append(a.Endpoint[i].Request[j].O, p)
			}
			sort.Sort(a.Endpoint[i].Request[j].O)

			for _, s := range a.Endpoint[i].Request[j].Response {
				p, ok := a.Response[s]
				if !ok {
					return fmt.Errorf("found no parameter for %s %s %s",
						a.Endpoint[i].Title,
						a.Endpoint[i].Request[j].Method,
						s)
				}
				a.Endpoint[i].Request[j].Res = append(a.Endpoint[i].Request[j].Res, p)
			}
			sort.Sort(a.Endpoint[i].Request[j].Res)
		}
	}

	return err
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
	b.WriteString(`"io/ioutil"` + "\n")
	b.WriteString(`)` + "\n")
	b.WriteString("\n")
	b.WriteString("var mux = http.NewServeMux()\n")
	b.WriteString("\n")

	// the init() func - add routes the mux

	b.WriteString("\n")
	b.WriteString("func init() {\n")

	b.WriteString(`mux.HandleFunc("/api-docs", weft.MakeHandlerPage(docHandler))` + "\n")

	for _, e := range a.Endpoint {
		b.WriteString(fmt.Sprintf("mux.HandleFunc(\"%s\", weft.MakeHandlerAPI(%s))\n", e.Uri, handlerName(e.Uri)))
	}

	b.WriteString("}\n")
	b.WriteString("\n")

	// the api-doc handler

	b.WriteString(`func docHandler(r *http.Request, h http.Header, b *bytes.Buffer) *weft.Result {` + "\n")
	b.WriteString(`switch r.Method {` + "\n")
	b.WriteString(`case "GET":` + "\n")
	b.WriteString(`by, err := ioutil.ReadFile("assets/api-docs/index.html")` + "\n")
	b.WriteString(`if err != nil {` + "\n")
	b.WriteString(`return weft.InternalServerError(err)` + "\n")
	b.WriteString(`}` + "\n")
	b.WriteString(`b.Write(by)` + "\n")
	b.WriteString(`return &weft.StatusOK` + "\n")
	b.WriteString(`default:` + "\n")
	b.WriteString(`return &weft.MethodNotAllowed` + "\n")
	b.WriteString(`}` + "\n")
	b.WriteString(`}` + "\n")

	// create the handler func for each endpoint with a method switch and an accept
	// switch for GET.

	for _, e := range a.Endpoint {
		b.WriteString(fmt.Sprintf("func %s(r *http.Request, h http.Header, b *bytes.Buffer) *weft.Result {\n", handlerName(e.Uri)))
		b.WriteString("switch r.Method {\n")

		get := e.Request.filter("GET")

		if len(get) > 0 {
			b.WriteString(`case "GET":` + "\n")
			b.WriteString(`switch r.Header.Get("Accept") {` + "\n")

			var d request
			var hasDefault bool

			for _, r := range get {
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

		put := e.Request.filter("PUT")

		if len(put) > 1 {
			return fmt.Errorf("found more than one PUT request for endpoint %s", e.Uri)
		}

		if len(put) == 1 {
			b.WriteString(`case "PUT":` + "\n")
			put[0].checkQuery(&b)
			b.WriteString(fmt.Sprintf("return %s(r, h, b)\n", put[0].Function))
		}

		delete := e.Request.filter("DELETE")

		if len(delete) > 1 {
			return fmt.Errorf("found more than one DELETE request for endpoint %s", e.Uri)
		}

		if len(delete) == 1 {
			b.WriteString(`case "DELETE":` + "\n")
			delete[0].checkQuery(&b)
			b.WriteString(fmt.Sprintf("return %s(r, h, b)\n", delete[0].Function))
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

func (a *api) writeDocs(filename string) error {
	b := new(bytes.Buffer)

	err := t.ExecuteTemplate(b, "index", a)
	if err != nil {
		return err
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

	return err
}
