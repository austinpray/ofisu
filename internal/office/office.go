package office

import (
	"bufio"
	"math"
	"os"
	"regexp"
	"strings"

	"github.com/agnivade/levenshtein"
	"golang.org/x/text/unicode/norm"
)

// Office is a graph of rooms
type Office struct {
	ID    string
	Name  string
	Rooms map[string]Room
	Edges map[string][]string
}

func (o *Office) GetAdjacentRooms(roomID string) []Room {
	availableMoves, ok := o.Edges[roomID]

	if !ok {
		return []Room{}
	}

	adjacent := []Room{}
	for _, availableMoves := range availableMoves {
		room, ok := o.Rooms[availableMoves]
		if !ok {
			return []Room{}
		}
		adjacent = append(adjacent, room)
	}

	return adjacent
}

func (o *Office) GetMoveCandidates(from string, desired string) []Room {
	desired = normalizeOfficeName(desired)

	adjacentRooms := o.GetAdjacentRooms(from)
	candidates := []Room{}

	for _, adj := range adjacentRooms {
		roomName := normalizeOfficeName(adj.Name)
		if strings.HasPrefix(roomName, desired) {
			candidates = append(candidates, adj)
			continue
		}

		// TODO: will need to tweak this
		// https://git.io/JLyEX
		threshold := int(math.Ceil(float64(len(roomName)) * 0.25))
		if levenshtein.ComputeDistance(desired, roomName) <= threshold {
			candidates = append(candidates, adj)
			continue
		}
	}

	return candidates

}

func normalizeOfficeName(raw string) string {
	out := norm.NFC.String(raw)
	out = strings.ToLower(out)
	// strip possessives
	out = strings.Replace(out, "'s", "", -1)

	out = strings.Join(strings.Fields(out), "_")
	return out
}

// Room is where players can hang out
type Room struct {
	ID           string
	Name         string
	Items        []string
	VoiceEnabled bool
}

var nodeRegex = `[_0-9a-zA-Z\200-\377]+`

var tGraph = regexp.MustCompile(`^graph\s+(.+)\s+{`)
var tEdge = regexp.MustCompile(`^(` + nodeRegex + `)\s+--\s+(` + nodeRegex + `)`)

/*
An ID is one of the following:
- Any string of alphabetic ([a-zA-Z\200-\377]) characters
- underscores ('_')
- digits ([0-9])
*/
var tNodeID = regexp.MustCompile(`^(` + nodeRegex + `)$`)

var tNodeName = regexp.MustCompile(`^# name: (.+)`)
var tNodeItem = regexp.MustCompile(`^# has: (.+)`)
var tNodeVoiceEnabled = regexp.MustCompile(`^# voice: (.+)`)

/*
FromFile parses an office file
An office file is basically just a graphviz dot file with special comments.
Currently we only support a basic `graph` with no subgraphs or anything like that.
We do _not_ support the full dot syntax. We only support basic edges like:

  A
  B
  C

  A -- B
  A -- C

If it makes sense, we can add a real dot parser. But as far as I can tell YAGNI.
*/
func FromFile(path string) (*Office, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	office := &Office{
		Rooms: make(map[string]Room),
		Edges: make(map[string][]string),
	}

	scanner := bufio.NewScanner(file)
	var itemBuf []string
	var currentName string
	var voiceEnabledBuf bool
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			itemBuf = []string{}
			currentName = ""
			voiceEnabledBuf = false
			continue
		}

		// capture name
		if matches := tNodeName.FindStringSubmatch(line); matches != nil {
			currentName = matches[1]
		}
		// capture items
		// TODO: could prolly support csv `has: a, b, c`
		if matches := tNodeItem.FindStringSubmatch(line); matches != nil {
			itemBuf = append(itemBuf, matches[1])
		}
		// enable voice
		if matches := tNodeVoiceEnabled.FindStringSubmatch(line); matches != nil {
			voiceEnabledBuf = matches[1] == "enabled"
		}

		/*
			standalone node with details
		*/
		if matches := tNodeID.FindStringSubmatch(line); matches != nil {
			roomID := matches[1]
			room, seen := office.Rooms[roomID]
			if !seen {
				room = Room{
					ID:   roomID,
					Name: roomID,
				}
			}
			if len(itemBuf) > 0 {
				room.Items = itemBuf
			}
			if currentName != "" {
				room.Name = currentName
			}
			if voiceEnabledBuf {
				room.VoiceEnabled = voiceEnabledBuf
			}
			itemBuf = []string{}
			currentName = ""
			voiceEnabledBuf = false
			office.Rooms[roomID] = room
		}

		/*
			Log an edge
		*/
		if matches := tEdge.FindStringSubmatch(line); matches != nil {
			for _, roomID := range matches[1:] {
				_, seen := office.Rooms[roomID]
				if !seen {
					office.Rooms[roomID] = Room{
						ID:   roomID,
						Name: roomID,
					}
				}
			}
			office.Edges[matches[1]] = append(office.Edges[matches[1]], matches[2])
			office.Edges[matches[2]] = append(office.Edges[matches[2]], matches[1])
		}

		/*
			grab office's ID and name
		*/
		if matches := tGraph.FindStringSubmatch(line); matches != nil {
			office.ID = matches[1]
			office.Name = office.ID
			if currentName != "" {
				office.Name = currentName
				currentName = ""
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return office, nil
}
