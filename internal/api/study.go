package api

// Study progress of the "Learn" section, kept per user so it follows the
// learner across browsers and devices: concepts understood and flashcards
// known per topic, best quiz score, practice-exam history and the practice
// exams in progress. The web client keeps a local copy as a cache.

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/neodevesp/gcp-trainer/internal/i18n"
	"github.com/neodevesp/gcp-trainer/internal/store"
)

const (
	maxExamHistory = 50
	maxRunBytes    = 256 << 10
	maxBodyBytes   = 512 << 10
)

// StudyTopic is the progress on one study topic.
type StudyTopic struct {
	Seen  []string `json:"seen,omitempty"`
	Known []string `json:"known,omitempty"`
	Best  *int     `json:"best,omitempty"`
	// Scores are the recent quiz results (oldest first, the last 12). The
	// latest one decides whether the topic is learnt, so a drop shows.
	Scores []QuizScore `json:"scores,omitempty"`
}

// QuizScore is one quiz result on a topic.
type QuizScore struct {
	At    int64 `json:"at"`
	Score int   `json:"score"`
}

const maxQuizScores = 12

// mergeScores keeps every distinct result of both sides (by time), so two
// devices never erase each other's quizzes.
func mergeScores(old, add []QuizScore) []QuizScore {
	seen := map[int64]bool{}
	var out []QuizScore
	for _, q := range append(append([]QuizScore{}, old...), add...) {
		if seen[q.At] || q.At <= 0 {
			continue
		}
		seen[q.At] = true
		q.Score = min(100, max(0, q.Score))
		out = append(out, q)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At < out[j].At })
	if len(out) > maxQuizScores {
		out = out[len(out)-maxQuizScores:]
	}
	return out
}

// StudyDomain is the result of one exam section in a practice exam.
type StudyDomain struct {
	Name  string `json:"name"`
	OK    int    `json:"ok"`
	Total int    `json:"total"`
}

// StudyExam is a finished practice exam.
type StudyExam struct {
	Cert    string        `json:"cert"`
	At      int64         `json:"at"`
	Score   int           `json:"score"`
	Total   int           `json:"total"`
	Full    bool          `json:"full"`
	Seconds int           `json:"seconds"`
	Domains []StudyDomain `json:"domains"`
}

// ReviewItem is a question in the spaced-repetition queue (Leitner boxes):
// a wrong answer puts it in box 1; each right answer moves it up a box and
// pushes its next review further away.
type ReviewItem struct {
	Box    int   `json:"box"`
	Due    int64 `json:"due"` // unix ms
	Misses int   `json:"misses"`
	At     int64 `json:"at"` // last update, unix ms (newest wins across devices)
}

// Study is the per-user document of the "study" collection.
type Study struct {
	User     string                     `json:"user"`
	Progress map[string]StudyTopic      `json:"progress"`
	Exams    []StudyExam                `json:"exams"`
	Runs     map[string]json.RawMessage `json:"runs"` // practice exams in progress, by client key
	// RunRevs counts the saves of each run. A client that sends the revision
	// it last saw (X-Run-Base) is refused with 409 and the current copy when
	// another device saved in between, so it can merge instead of overwrite.
	RunRevs map[string]int64      `json:"runRevs,omitempty"`
	Review  map[string]ReviewItem `json:"review"` // spaced repetition, by "topic:q3" / "topic:s1"
	Updated time.Time             `json:"updated"`
}

const maxReviewItems = 3000

// httpErrBody is the current copy of a run returned with a 409.
type httpErrBody struct {
	Run json.RawMessage
	Rev int64
}

var reviewKey = regexp.MustCompile(`^([a-z0-9-]+):[qs]\d{1,3}$`)

// Practice exams in progress, the support shift and the 10-day plan checklist.
var runKey = regexp.MustCompile(`^(cm-exam-run-[A-Z]{2,6}-(full|quick)|cm-shift|cm-plan|cm-final|cm-onboarding)$`)

func (s *Server) loadStudy(user string) (*Study, error) {
	st := &Study{User: user}
	if err := s.Store.Get("study", user, st); err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if st.Progress == nil {
		st.Progress = map[string]StudyTopic{}
	}
	if st.Runs == nil {
		st.Runs = map[string]json.RawMessage{}
	}
	if st.Exams == nil {
		st.Exams = []StudyExam{}
	}
	if st.Review == nil {
		st.Review = map[string]ReviewItem{}
	}
	return st, nil
}

// updateStudy applies f to the user's study document under a lock and saves it.
func (s *Server) updateStudy(user string, f func(*Study) error) (*Study, error) {
	s.studyMu.Lock()
	defer s.studyMu.Unlock()
	st, err := s.loadStudy(user)
	if err != nil {
		return nil, err
	}
	if err := f(st); err != nil {
		return nil, err
	}
	st.Updated = time.Now().UTC()
	return st, s.Store.Put("study", user, st)
}

func decodeBody(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes)).Decode(v)
}

// cleanIDs keeps the known concept ids of a topic, without duplicates.
func (s *Server) cleanIDs(topic string, ids []string) []string {
	valid := map[string]bool{}
	for _, t := range s.Cat.Study {
		if t.ID == topic {
			for _, c := range t.Concepts {
				valid[c.ID] = true
			}
		}
	}
	out := []string{}
	seen := map[string]bool{}
	for _, id := range ids {
		if valid[id] && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func (s *Server) validTopic(id string) bool {
	for _, t := range s.Cat.Study {
		if t.ID == id {
			return true
		}
	}
	return false
}

func (s *Server) validCert(id string) bool {
	for _, e := range s.Cat.Exams {
		if e.Cert == id {
			return true
		}
	}
	return false
}

func (s *Server) studyRoutes(h func(string, func(http.ResponseWriter, *http.Request) (any, error), ...string)) {
	bad := func(r *http.Request, es, en string) error { return httpErr{400, i18n.P(s.lang(r), es, en)} }

	h("GET /api/me/study", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return s.loadStudy(userOf(r).ID)
	})

	// Replace the progress of one or more topics. A best score only ever rises,
	// so an older device cannot lower it.
	h("POST /api/me/study/progress", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req map[string]StudyTopic
		if err := decodeBody(r, &req); err != nil || len(req) == 0 || len(req) > 100 {
			return nil, bad(r, "progreso no válido", "invalid progress")
		}
		st, err := s.updateStudy(userOf(r).ID, func(st *Study) error {
			for id, p := range req {
				if !s.validTopic(id) {
					return bad(r, "tema desconocido: "+id, "unknown topic: "+id)
				}
				p.Seen, p.Known = s.cleanIDs(id, p.Seen), s.cleanIDs(id, p.Known)
				if p.Best != nil {
					b := min(100, max(0, *p.Best))
					if old := st.Progress[id].Best; old != nil && *old > b {
						b = *old
					}
					p.Best = &b
				} else {
					p.Best = st.Progress[id].Best
				}
				p.Scores = mergeScores(st.Progress[id].Scores, p.Scores)
				st.Progress[id] = p
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"progress": st.Progress}, nil
	})

	// Record a finished practice exam; the run it came from is discarded.
	h("POST /api/me/study/exams", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var e StudyExam
		if err := decodeBody(r, &e); err != nil || !s.validCert(e.Cert) || e.Total < 1 || e.Total > 100 || e.Score < 0 || e.Score > e.Total || e.Seconds < 0 || len(e.Domains) > 12 {
			return nil, bad(r, "simulacro no válido", "invalid practice exam")
		}
		if e.At == 0 {
			e.At = time.Now().UnixMilli()
		}
		st, err := s.updateStudy(userOf(r).ID, func(st *Study) error {
			for _, x := range st.Exams {
				if x.At == e.At && x.Cert == e.Cert {
					return nil // already recorded (retried upload)
				}
			}
			st.Exams = append([]StudyExam{e}, st.Exams...)
			sort.SliceStable(st.Exams, func(i, j int) bool { return st.Exams[i].At > st.Exams[j].At })
			if len(st.Exams) > maxExamHistory {
				st.Exams = st.Exams[:maxExamHistory]
			}
			mode := "quick"
			if e.Full {
				mode = "full"
			}
			delete(st.Runs, "cm-exam-run-"+e.Cert+"-"+mode)
			return nil
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"exams": st.Exams}, nil
	})

	// Update the spaced-repetition queue: each key maps to its new state, or
	// to null to drop it (mastered). The newest update of an item wins.
	h("POST /api/me/study/review", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req map[string]*ReviewItem
		if err := decodeBody(r, &req); err != nil || len(req) == 0 || len(req) > 200 {
			return nil, bad(r, "repaso no válido", "invalid review")
		}
		for k, it := range req {
			m := reviewKey.FindStringSubmatch(k)
			if m == nil || !s.validTopic(m[1]) || (it != nil && (it.Box < 1 || it.Box > 6 || it.Misses < 0)) {
				return nil, bad(r, "pregunta de repaso no válida: "+k, "invalid review question: "+k)
			}
		}
		st, err := s.updateStudy(userOf(r).ID, func(st *Study) error {
			for k, it := range req {
				if old, ok := st.Review[k]; ok && it != nil && old.At > it.At {
					continue // an older device must not overwrite a newer answer
				}
				if it == nil {
					delete(st.Review, k)
				} else if _, ok := st.Review[k]; ok || len(st.Review) < maxReviewItems {
					st.Review[k] = *it
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"review": st.Review}, nil
	})

	// Save or discard a practice exam in progress so it can be resumed elsewhere.
	h("PUT /api/me/study/runs/{key}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		key := r.PathValue("key")
		if !runKey.MatchString(key) {
			return nil, bad(r, "clave no válida", "invalid key")
		}
		var raw json.RawMessage
		if err := decodeBody(r, &raw); err != nil || len(raw) > maxRunBytes || !json.Valid(raw) {
			return nil, bad(r, "simulacro no válido", "invalid practice exam")
		}
		var rev int64
		var conflict *httpErrBody
		_, err := s.updateStudy(userOf(r).ID, func(st *Study) error {
			if st.RunRevs == nil {
				st.RunRevs = map[string]int64{}
			}
			cur := st.RunRevs[key]
			if base := r.Header.Get("X-Run-Base"); base != "" {
				if b, perr := strconv.ParseInt(base, 10, 64); perr == nil && b != cur && st.Runs[key] != nil {
					conflict = &httpErrBody{Run: st.Runs[key], Rev: cur}
					return nil
				}
			}
			st.Runs[key] = raw
			rev = cur + 1
			st.RunRevs[key] = rev
			return nil
		})
		if err != nil {
			return nil, err
		}
		if conflict != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": i18n.P(s.lang(r), "otro dispositivo guardó antes: se han combinado los cambios", "another device saved first: changes were merged"), "run": conflict.Run, "rev": conflict.Rev})
			return nil, nil
		}
		return map[string]any{"ok": true, "rev": rev}, nil
	})
	h("DELETE /api/me/study/runs/{key}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		key := r.PathValue("key")
		_, err := s.updateStudy(userOf(r).ID, func(st *Study) error {
			delete(st.Runs, key)
			if st.RunRevs != nil {
				st.RunRevs[key]++
			}
			return nil
		})
		return map[string]bool{"ok": err == nil}, err
	})
}
