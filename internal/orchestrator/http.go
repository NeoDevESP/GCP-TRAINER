package orchestrator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/neodevesp/gcp-trainer/internal/cli"
	"github.com/neodevesp/gcp-trainer/internal/grader"
	"github.com/neodevesp/gcp-trainer/internal/scenario"
)

// Handler exposes a LabPlane over HTTP (internal network only, authenticated
// with a shared bearer token between planes).
func Handler(lp LabPlane, token string) http.Handler {
	mux := http.NewServeMux()
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if token != "" && r.Header.Get("Authorization") != "Bearer "+token {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			h(w, r)
		}
	}
	reply := func(w http.ResponseWriter, v any, err error) {
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("POST /lab/sessions", auth(func(w http.ResponseWriter, r *http.Request) {
		var req StartRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		info, err := lp.Start(req)
		reply(w, info, err)
	}))
	mux.HandleFunc("GET /lab/sessions/{id}", auth(func(w http.ResponseWriter, r *http.Request) {
		info, err := lp.Info(r.PathValue("id"))
		reply(w, info, err)
	}))
	mux.HandleFunc("POST /lab/sessions/{id}/exec", auth(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Line string }
		_ = json.NewDecoder(r.Body).Decode(&req)
		res, err := lp.Exec(r.PathValue("id"), req.Line)
		reply(w, res, err)
	}))
	mux.HandleFunc("GET /lab/sessions/{id}/views/{kind}", auth(func(w http.ResponseWriter, r *http.Request) {
		params := map[string]string{}
		for k := range r.URL.Query() {
			params[k] = r.URL.Query().Get(k)
		}
		v, err := lp.View(r.PathValue("id"), r.PathValue("kind"), params)
		reply(w, v, err)
	}))
	mux.HandleFunc("PUT /lab/sessions/{id}/files", auth(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Path, Content string }
		_ = json.NewDecoder(r.Body).Decode(&req)
		reply(w, map[string]bool{"ok": true}, lp.PutFile(r.PathValue("id"), req.Path, req.Content))
	}))
	mux.HandleFunc("POST /lab/sessions/{id}/grade", auth(func(w http.ResponseWriter, r *http.Request) {
		var sub grader.Submission
		_ = json.NewDecoder(r.Body).Decode(&sub)
		res, err := lp.Grade(r.PathValue("id"), sub)
		reply(w, res, err)
	}))
	mux.HandleFunc("POST /lab/sessions/{id}/hint", auth(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ N int }
		_ = json.NewDecoder(r.Body).Decode(&req)
		h, err := lp.Hint(r.PathValue("id"), req.N)
		reply(w, h, err)
	}))
	mux.HandleFunc("GET /lab/sessions/{id}/export", auth(func(w http.ResponseWriter, r *http.Request) {
		b, err := lp.Export(r.PathValue("id"))
		reply(w, map[string]json.RawMessage{"state": b}, err)
	}))
	mux.HandleFunc("POST /lab/sessions/{id}/stop", auth(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Reason string }
		_ = json.NewDecoder(r.Body).Decode(&req)
		reply(w, map[string]bool{"ok": true}, lp.Stop(r.PathValue("id"), req.Reason))
	}))
	return mux
}

// Client is a LabPlane backed by a remote lab-plane service.
type Client struct {
	Base  string
	Token string
	HTTP  *http.Client
}

func (c *Client) do(method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, strings.TrimSuffix(c.Base, "/")+path, body)
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 5 * time.Minute}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		var e struct{ Error string }
		_ = json.Unmarshal(b, &e)
		if e.Error == "" {
			e.Error = string(b)
		}
		return fmt.Errorf("%s", e.Error)
	}
	if out != nil {
		return json.Unmarshal(b, out)
	}
	return nil
}

func (c *Client) Start(req StartRequest) (*SessionInfo, error) {
	var out SessionInfo
	return &out, c.do("POST", "/lab/sessions", req, &out)
}
func (c *Client) Info(id string) (*SessionInfo, error) {
	var out SessionInfo
	return &out, c.do("GET", "/lab/sessions/"+id, nil, &out)
}
func (c *Client) Exec(id, line string) (cli.Result, error) {
	var out cli.Result
	return out, c.do("POST", "/lab/sessions/"+id+"/exec", map[string]string{"Line": line}, &out)
}
func (c *Client) View(id, kind string, params map[string]string) (any, error) {
	q := url.Values{}
	for k, v := range params {
		q.Set(k, v)
	}
	var out any
	return out, c.do("GET", "/lab/sessions/"+id+"/views/"+kind+"?"+q.Encode(), nil, &out)
}
func (c *Client) PutFile(id, path, content string) error {
	return c.do("PUT", "/lab/sessions/"+id+"/files", map[string]string{"Path": path, "Content": content}, nil)
}
func (c *Client) Grade(id string, sub grader.Submission) (*grader.Result, error) {
	var out grader.Result
	return &out, c.do("POST", "/lab/sessions/"+id+"/grade", sub, &out)
}
func (c *Client) Stop(id, reason string) error {
	return c.do("POST", "/lab/sessions/"+id+"/stop", map[string]string{"Reason": reason}, nil)
}
func (c *Client) Hint(id string, n int) (*scenario.Hint, error) {
	var out scenario.Hint
	return &out, c.do("POST", "/lab/sessions/"+id+"/hint", map[string]int{"N": n}, &out)
}
func (c *Client) Export(id string) ([]byte, error) {
	var out struct {
		State json.RawMessage `json:"state"`
	}
	err := c.do("GET", "/lab/sessions/"+id+"/export", nil, &out)
	return out.State, err
}
