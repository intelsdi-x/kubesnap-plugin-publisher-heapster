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

package publisher

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"

	log "github.com/Sirupsen/logrus"

	"encoding/json"
	"github.com/intelsdi-x/kubesnap-plugin-publisher-heapster/exchange"
	"github.com/intelsdi-x/kubesnap-plugin-publisher-heapster/server"
	"github.com/intelsdi-x/kubesnap-plugin-publisher-heapster/util"
	"github.com/intelsdi-x/snap/control/plugin"
	"github.com/intelsdi-x/snap/control/plugin/cpolicy"
	"github.com/intelsdi-x/snap/core/ctypes"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	name       = "heapster"
	version    = 1
	pluginType = plugin.PublisherPluginType
)

const (
	dockerMetricPrefix = "/intel/linux/docker"
	defStatsDepth      = 10
	defServerPort      = 8777
	defStatsSpanStr    = "10m"
	defStatsSpan       = 10 * time.Minute
	defExportTmplFile  = "builtin"
	cfgStatsDepth      = "stats_depth"
	cfgServerPort      = "server_port"
	cfgStatsSpan       = "stats_span"
	cfgExportTmplFile  = "export_tmpl_file"
)

type core struct {
	logger         *log.Logger
	state          *exchange.InnerState
	once           sync.Once
	statsDepth     int
	statsSpan      time.Duration
	exportTmplFile string
	metricTemplate MetricTemplate
}

type ConfigMap map[string]ctypes.ConfigValue

func NewInnerState() *exchange.InnerState {
	res := &exchange.InnerState{
		DockerPaths:   map[string]string{},
		DockerStorage: map[string]interface{}{},
	}
	return res
}

func NewCore() (*core, error) {
	log.SetOutput(os.Stderr)
	logger := log.New()
	core := core{
		state:      NewInnerState(),
		logger:     logger,
		statsDepth: defStatsDepth,
		statsSpan:  defStatsSpan,
	}
	return &core, nil
}

func (f *core) Publish(contentType string, content []byte, config map[string]ctypes.ConfigValue) error {
	f.ensureInitialized(config)
	var metrics []plugin.MetricType

	switch contentType {
	case plugin.SnapGOBContentType:
		dec := gob.NewDecoder(bytes.NewBuffer(content))
		if err := dec.Decode(&metrics); err != nil {
			f.logger.Printf("Error decoding: error=%v content=%v", err, content)
			return err
		}
	default:
		f.logger.Printf("Error unknown content type '%v'", contentType)
		return errors.New(fmt.Sprintf("Unknown content type '%s'", contentType))
	}
	f.state.Lock()
	defer f.state.Unlock()
	f.processMetrics(metrics)
	return nil
}

func Meta() *plugin.PluginMeta {
	return plugin.NewPluginMeta(
		name, version, pluginType,
		[]string{plugin.SnapGOBContentType},
		[]string{plugin.SnapGOBContentType},
		plugin.ConcurrencyCount(999))
}

func (f *core) GetConfigPolicy() (*cpolicy.ConfigPolicy, error) {
	cp := cpolicy.New()
	p := cpolicy.NewPolicyNode()
	rule1, _ := cpolicy.NewIntegerRule(cfgServerPort, false, defServerPort)
	rule2, _ := cpolicy.NewIntegerRule(cfgStatsDepth, false, defStatsDepth)
	rule3, _ := cpolicy.NewStringRule(cfgStatsSpan, false, defStatsSpanStr)
	rule4, _ := cpolicy.NewStringRule(cfgExportTmplFile, false, defExportTmplFile)
	p.Add(rule1, rule2, rule3, rule4)
	cp.Add([]string{}, p)
	return cp, nil
}

func (m ConfigMap) GetInt(key string, defValue int) int {
	if value, gotIt := m[key]; gotIt {
		return value.(ctypes.ConfigValueInt).Value
	} else {
		return defValue
	}
}

func (m ConfigMap) GetStr(key string, defValue string) string {
	if value, gotIt := m[key]; gotIt {
		return value.(ctypes.ConfigValueStr).Value
	} else {
		return defValue
	}
}

func (f *core) ensureInitialized(config map[string]ctypes.ConfigValue) {
	configMap := ConfigMap(config)
	f.once.Do(func() {
		defer func() {
			if r := recover(); r != nil {
				f.logger.Errorf("Caught an error: %s", r)
			}
		}()
		f.statsDepth = configMap.GetInt(cfgStatsDepth, defStatsDepth)
		serverPort := configMap.GetInt(cfgServerPort, defServerPort)
		statsSpanStr := configMap.GetStr(cfgStatsSpan, defStatsSpanStr)
		if statsSpan, err := time.ParseDuration(statsSpanStr); err != nil {
			f.statsSpan = defStatsSpan
		} else {
			f.statsSpan = statsSpan
		}
		f.exportTmplFile = configMap.GetStr(cfgExportTmplFile, defExportTmplFile)
		if err := f.loadMetricTemplate(); err != nil {
			f.logger.Fatalf("couldn't load metric template: %s", err)
		}
		server.EnsureStarted(f.state, serverPort)
	})
}

type MetricTemplate struct {
	source      string
	statsSource string
	mapToStats  map[string]string
	mapToDocker map[string]string
}

func (f *core) loadMetricTemplate() error {
	var err error
	var source string
	if source, err = f.loadTemplateSource(); err != nil {
		return err
	}
	var templateRef interface{}
	// parse template once for test
	if err = json.Unmarshal([]byte(source), &templateRef); err != nil {
		return err
	}
	templateObj := templateRef.(map[string]interface{})
	extractMapping := func(obj interface{}) map[string]string {
		mapping := map[string]string{}
		tmplWalker := util.NewObjWalker(obj)
		tmplWalker.Walk("/", func(target string, info os.FileInfo, _ error) error {
			if source, isStr := info.Sys().(string); isStr &&
				strings.HasPrefix(source, "/") {
				mapping[source] = target
			}
			return nil
		})
		return mapping
	}
	statsListRef, _ := util.NewObjWalker(templateObj).Seek("/stats")
	statsList := statsListRef.([]interface{})
	var statsObj interface{}
	statsObj, statsList = statsList[0], statsList[1:]
	map[string]interface{}(templateObj)["stats"] = statsList
	statsTemplate, _ := json.Marshal(statsObj)
	dockerTemplate, _ := json.Marshal(templateObj)
	f.metricTemplate = MetricTemplate{
		source:      string(dockerTemplate),
		statsSource: string(statsTemplate),
		mapToStats:  extractMapping(statsObj),
		mapToDocker: extractMapping(templateObj),
	}
	return nil
}

func (f *core) loadTemplateSource() (string, error) {
	if f.exportTmplFile == defExportTmplFile {
		templateSrc := builtinMetricTemplate
		return templateSrc, nil
	} else if templateSrc, err := ioutil.ReadFile(f.exportTmplFile); err != nil {
		return "", err
	} else {
		return string(templateSrc), nil
	}
}

func (f *core) extractDockerIdAndPath(metric *plugin.MetricType) (string, string, bool) {
	ns := metric.Namespace().String()
	if !strings.HasPrefix(ns, dockerMetricPrefix) {
		return "", "", false
	}
	tailSplit := strings.Split(strings.TrimLeft(strings.TrimPrefix(ns, dockerMetricPrefix), "/"), "/")
	id := tailSplit[0]
	path := "/" + id
	return id, path, true
}

func (f *core) processMetrics(metrics []plugin.MetricType) {
	dockerPaths := f.state.DockerPaths
	dockerStorage := f.state.DockerStorage
	temporaryStats := map[string]map[string]interface{}{}
	fetchObjectForDocker := func(id, path string, metric *plugin.MetricType) (obj map[string]interface{}, existedBefore bool) {
		//TODO: support the docker tree
		if dockerObj, gotIt := dockerStorage[path]; gotIt {
			dockerMap := dockerObj.(map[string]interface{})
			return dockerMap, true
		} else {
			dockerPaths[path] = id
			var dockerMap map[string]interface{}
			json.Unmarshal([]byte(f.metricTemplate.source), &dockerMap)
			dockerMap["name"] = path
			dockerMap["id"] = id

			dockerStorage[path] = dockerMap
			return dockerMap, false
		}
	}
	fetchObjectForStats := func(id, path string, metric *plugin.MetricType) (map[string]interface{}, bool) {
		var statsObj map[string]interface{}
		var haveStats bool
		if statsObj, haveStats = temporaryStats[path]; haveStats {
			return statsObj, true
		} else if metric != nil {
			json.Unmarshal([]byte(f.metricTemplate.statsSource), &statsObj)
			statsObj["timestamp"] = metric.Timestamp().String()
			temporaryStats[path] = statsObj
			return statsObj, true
		} else {
			return statsObj, false
		}
	}
	validateStatsMetric := func(path, ns string) (string, bool) {
		for sourcePath, _ := range f.metricTemplate.mapToStats {
			if strings.HasSuffix(ns, sourcePath) {
				return sourcePath, true
			}
		}
		customPath := ns[strings.LastIndex(ns, path)+len(path):]
		return customPath, false
	}
	validateDockerMetric := func(path, ns string) (string, bool) {
		for sourcePath, _ := range f.metricTemplate.mapToDocker {
			if strings.HasSuffix(ns, sourcePath) {
				return sourcePath, true
			}
		}
		return "", false
	}
	insertIntoStats := func(dockerPath string, statsObj map[string]interface{}, metric *plugin.MetricType) (didInsert bool) {
		ns := metric.Namespace().String()
		didInsert = false
		if sourcePath, isStatsMetric := validateStatsMetric(dockerPath, ns); isStatsMetric {
			targetPath := f.metricTemplate.mapToStats[sourcePath]
			metricParent, _ := util.NewObjWalker(statsObj).Seek(filepath.Dir(targetPath))
			metricParentMap := metricParent.(map[string]interface{})
			metricParentMap[filepath.Base(targetPath)] = metric.Data()
			didInsert = true
		} else {
			//TODO: handle custom metrics
			//snapMetricsList, _ := util.NewObjWalker(statsObj).Seek("/stats/custom_metrics/SNAP")
			//oneMetric := map[string]interface{} {}
			//oneMetric["name"] = dockerMetricPrefix + "/"+ sourcePath
			//oneMetric["type"] = "gauge"
			//oneMetric[""]
		}
		return
	}

	insertIntoDocker := func(dockerPath string, dockerObj map[string]interface{}, metric *plugin.MetricType) (didInsert bool) {
		ns := metric.Namespace().String()
		didInsert = false
		sourcePath, isDockerMetric := validateDockerMetric(dockerPath, ns)
		if !isDockerMetric {
			return
		}
		targetPath := f.metricTemplate.mapToDocker[sourcePath]
		metricParent, _ := util.NewObjWalker(dockerObj).Seek(filepath.Dir(targetPath))
		metricParentMap := metricParent.(map[string]interface{})
		metricParentMap[filepath.Base(targetPath)] = metric.Data()
		didInsert = true
		return
	}

	mergeStatsForDocker := func(id, path string) {
		dockerObj, _ := fetchObjectForDocker(id, path, nil)
		statsObj, haveStats := fetchObjectForStats(id, path, nil)
		if !haveStats {
			return
		}
		statsList := dockerObj["stats"].([]interface{})
		// make sure we don't overflow  statsDepth nor  statsSpan when
		//new  statsObj is added
		makeRoomForStats := func() {
			validOfs := 0
			if f.statsDepth > 0 && len(statsList) == f.statsDepth {
				validOfs++
			}
			if f.statsSpan <= 0 {
				statsList = statsList[:copy(statsList, statsList[validOfs:])]
				return
			}
			nuStamp, _ := util.ParseTime(statsObj["timestamp"].(string))
			for validOfs < len(statsList) {
				ckStamp, _ := util.ParseTime(statsList[validOfs].(map[string]interface{})["timestamp"].(string))
				span := nuStamp.Sub(ckStamp)
				if span <= f.statsSpan {
					break
				}
				validOfs++
			}
			statsList = statsList[:copy(statsList, statsList[validOfs:])]
		}
		makeRoomForStats()
		statsList = append(statsList, statsObj)
		dockerObj["stats"] = statsList
	}
	firstTimeDockers := map[string]bool{}
	for _, mt := range metrics {
		if id, path, isDockerMetric := f.extractDockerIdAndPath(&mt); isDockerMetric {
			dockerObj, knownDocker := fetchObjectForDocker(id, path, &mt)
			if !knownDocker {
				firstTimeDockers[path] = true
			}
			statsObj, _ := fetchObjectForStats(id, path, &mt)
			if !insertIntoStats(path, statsObj, &mt) {
				if _, firstTimeDocker := firstTimeDockers[path]; firstTimeDocker {
					insertIntoDocker(path, dockerObj, &mt)
				}
			}
		}
	}
	for path, id := range dockerPaths {
		mergeStatsForDocker(id, path)
	}
}
