/*
http://www.apache.org/licenses/LICENSE-2.0.txt


Copyright 2016 Intel Corporation

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package server

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"net/http"

	"encoding/json"
	"github.com/gorilla/mux"
	"io"
	"io/ioutil"

	"github.com/intelsdi-x/kubesnap-plugin-publisher-heapster/exchange"
	"github.com/intelsdi-x/kubesnap-plugin-publisher-heapster/util"
	"sync"
	"time"
	"sort"
	"os"
)

var logger *log.Logger
var once sync.Once

type server struct {
	state *exchange.InnerState
	port int
	stats serverStats
}

type serverStats struct {
	statsTxMax	int
	statsTxTotal	int
	statsTxLast	int
	statsDdMax	int
	statsDdTotal	int
	statsDdLast	int
}

func EnsureStarted(state *exchange.InnerState, port int) error {
        var err error
	once.Do(func() {
		server := server{state: state, port: port}
                go func () { err = ServerFunc(&server) }()
	})
        return err
}

func ServerFunc(server *server) error {
	log.SetOutput(os.Stderr)
	logger = log.New()
	router := mux.NewRouter().StrictSlash(true)
	router.Methods("POST").Path("/stats/container/").HandlerFunc(wrapper(server, Stats))
        err := http.ListenAndServe(fmt.Sprintf(":%d", server.port), router)
        return err
}

func copyFlat(data map[string]interface{}) map[string]interface{} {
	res := map[string]interface{}{}
	for k, v := range data {
		res[k] = v
	}
	return res
}

type statsListType []interface{}

func (s statsListType) Len() int {
	return len(s)
}

func (s statsListType) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s statsListType) Less(i, j int) bool {
	l := s[i].(map[string]interface{})
	r := s[j].(map[string]interface{})
	a, _ := util.ParseTime(l["timestamp"].(string))
	b, _ := util.ParseTime(r["timestamp"].(string))
	// allow reverse order - most recent first
	return !a.Before(b)
}

func buildStatsResponse(server *server, stats *exchange.StatsRequest) (interface{}) {
	state := server.state
	state.RLock()
	defer state.RUnlock()
	ref := state.DockerStorage
	res := map[string]map[string]interface{}{}
	stats_statsTx := 0
	stats_statsDd := 0
	for dockerName, dockerObj := range ref {
		dockerCopy := copyFlat(dockerObj.(map[string]interface{}))
		statsList := dockerCopy["stats"].([]interface{})
		statsSorted := make([]interface{}, 0, len(statsList))
		for _, statsObj := range statsList {
			statsSorted = append(statsSorted, statsObj)
		}
		sort.Sort(statsListType(statsSorted))
		statsCopy := make([]interface{}, 0, len(statsSorted))
		for _, statsObj := range statsSorted {
			statsMap := statsObj.(map[string]interface{})
			ckStamp, _ := util.ParseTime(statsMap["timestamp"].(string))
			if ckStamp.Before(stats.Start) || ckStamp.After(stats.End) {
				stats_statsDd++
				continue
			}
			statsCopy = append(statsCopy, statsObj)
			stats_statsTx++
			if stats.NumStats > 0 && len(statsCopy) >= stats.NumStats {
				break
			}
		}
		dockerCopy["stats"] = statsCopy
		res[dockerName] = dockerCopy
	}
	// update the statistics
	if stats_statsDd > server.stats.statsDdMax {
		server.stats.statsDdMax = stats_statsDd
	}
	server.stats.statsDdLast = stats_statsDd
	server.stats.statsDdTotal += stats_statsDd
	if stats_statsTx > server.stats.statsTxMax {
		server.stats.statsTxMax = stats_statsTx
	}
	server.stats.statsTxLast = stats_statsTx
	server.stats.statsTxTotal += stats_statsTx
	return res
}

func Stats(server *server, w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, 1048576))
	if err != nil {
		panic(err)
	}
	if err := r.Body.Close(); err != nil {
		panic(err)
	}
	var statsJson map[string]interface{}
	if err := json.Unmarshal(body, &statsJson); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.WriteHeader(422)
		if err := json.NewEncoder(w).Encode(err); err != nil {
			panic(err)
		}
		return
	}
	var stats exchange.StatsRequest
	json.Unmarshal(body, &stats)
	if _, gotStart := statsJson["start"]; !gotStart {
		stats.Start = time.Time{}
	}
	if _, gotEnd := statsJson["end"]; !gotEnd {
		stats.End = time.Now()
	}
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	res := buildStatsResponse(server, &stats)
	//logger.Infof("Received request: %+v; current time in seconds: %v, current time: %s, processing stats: %+v", stats, time.Now().Unix(), time.Now(), server.stats)
	if err := json.NewEncoder(w).Encode(res); err != nil {
		panic(err)
	}
}

func wrapper(server *server, fu func(*server, http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		Stats(server, w, r)
	}
}
