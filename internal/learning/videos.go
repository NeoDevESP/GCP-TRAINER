package learning

// Videos are official Google Cloud videos (YouTube) attached to study
// topics, optionally to one concept (content/learn/videos.yaml).

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

// Video is a YouTube video shown with a topic.
type Video struct {
	ID      string `yaml:"id" json:"id"`
	Title   string `yaml:"title" json:"title"`
	Channel string `yaml:"channel" json:"channel"`
	Length  string `yaml:"length" json:"length"`
	Concept string `yaml:"concept" json:"concept,omitempty"`
}

var youtubeID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

func loadVideos(path string) (map[string][]Video, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string][]Video{}, nil
	}
	if err != nil {
		return nil, err
	}
	var doc struct {
		Videos map[string][]Video `yaml:"videos"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if doc.Videos == nil {
		doc.Videos = map[string][]Video{}
	}
	return doc.Videos, nil
}

func (c *Catalog) validateVideos() []string {
	var problems []string
	topics := map[string]map[string]bool{}
	for _, t := range c.Study {
		cs := map[string]bool{}
		for _, k := range t.Concepts {
			cs[k.ID] = true
		}
		topics[t.ID] = cs
	}
	for topic, vs := range c.Videos {
		cs, ok := topics[topic]
		if !ok {
			problems = append(problems, fmt.Sprintf("videos: unknown topic %s", topic))
			continue
		}
		for _, v := range vs {
			if !youtubeID.MatchString(v.ID) || v.Title == "" {
				problems = append(problems, fmt.Sprintf("videos %s: bad video %q", topic, v.ID))
			}
			if v.Concept != "" && !cs[v.Concept] {
				problems = append(problems, fmt.Sprintf("videos %s/%s: unknown concept %s", topic, v.ID, v.Concept))
			}
		}
	}
	return problems
}
