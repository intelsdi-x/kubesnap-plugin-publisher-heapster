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
)

var logger *log.Logger
var once sync.Once

func EnsureStarted(state *exchange.InnerState, port int) {
	once.Do(func() {
		go ServerFunc(state, port)
	})
}

func ServerFunc(state *exchange.InnerState, port int) {
	logger = log.New()
	router := mux.NewRouter().StrictSlash(true)
	router.Methods("POST").Path("/stats/container/").HandlerFunc(wrapper(state, Stats))
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), router))
}

func copyFlat(data map[string]interface{}) map[string]interface{} {
	res := map[string]interface{}{}
	for k, v := range data {
		res[k] = v
	}
	return res
}

func buildStatsResponse(state *exchange.InnerState, stats *exchange.StatsRequest) interface{} {
	state.RLock()
	defer state.RUnlock()
	ref := state.DockerStorage
	res := map[string]map[string]interface{}{}
	for dockerName, dockerObj := range ref {
		dockerCopy := copyFlat(dockerObj.(map[string]interface{}))
		statsList := dockerCopy["stats"].([]interface{})
		statsCopy := make([]interface{}, 0, len(statsList))
		for _, statsObj := range statsList {
			statsMap := statsObj.(map[string]interface{})
			ckStamp, _ := util.ParseTime(statsMap["timestamp"].(string))
			if ckStamp.Before(stats.Start) || ckStamp.After(stats.End) {
				continue
			}
			statsCopy = append(statsCopy, statsObj)
			if stats.NumStats > 0 && len(statsCopy) >= stats.NumStats {
				break
			}
		}
		dockerCopy["stats"] = statsCopy
		res[dockerName] = dockerCopy
	}
	return res
}

func Stats(state *exchange.InnerState, w http.ResponseWriter, r *http.Request) {
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
	res := buildStatsResponse(state, &stats)
	if err := json.NewEncoder(w).Encode(res); err != nil {
		panic(err)
	}
}

func wrapper(state *exchange.InnerState, fu func(*exchange.InnerState, http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		Stats(state, w, r)
	}
}
