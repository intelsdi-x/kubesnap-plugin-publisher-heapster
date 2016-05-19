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
	"github.com/satori/go.uuid"
)

const (
	name       = "heapster"
	version    = 3
	pluginType = plugin.PublisherPluginType
)

const (
	dockerMetricPrefix = "/intel/docker"
	defStatsDepth      = 10
	defServerPort      = 8777
	defStatsSpanStr    = "10m"
	defStatsSpan       = 10 * time.Minute
	defExportTmplFile  = "builtin"
	defTstampDeltaStr  = "0"
	defTstampDelta     = 0
	cfgStatsDepth      = "stats_depth"
	cfgServerPort      = "server_port"
	cfgStatsSpan       = "stats_span"
	cfgExportTmplFile  = "export_tmpl_file"
	cfgTstampDelta     = "timestamp_delta"
)

type coreStats struct {
	metricsRxTotal       int
	metricsRxRecently    int
	containersRxRecently int
	containersRxMax      int
	statsRxRecently      int
	statsRxMax           int
	statsRxTotal         int
}

type core struct {
	logger         *log.Logger
	state          *exchange.InnerState
	once           sync.Once
	statsDepth     int
	statsSpan      time.Duration
	exportTmplFile string
	tstampDelta    time.Duration
	metricTemplate MetricTemplate
	stats          coreStats
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
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "Defeated by errors in Init: %s, %#v", r, r)
			panic(r)
		}
	}()
	logger := log.New()
	core := core{
		state:      NewInnerState(),
		logger:     logger,
		statsDepth: defStatsDepth,
		statsSpan:  defStatsSpan,
		stats:      coreStats{},
	}
	return &core, nil
}

func (f *core) Publish(contentType string, content []byte, config map[string]ctypes.ConfigValue) error {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "Defeated by errors in Publish: %s, %#v", r, r)
			panic(r)
		}
	}()
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
	rule5, _ := cpolicy.NewStringRule(cfgTstampDelta, false, defTstampDeltaStr)
	p.Add(rule1, rule2, rule3, rule4, rule5)
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
		tstampDeltaStr := configMap.GetStr(cfgTstampDelta, defTstampDeltaStr)
		tstampDelta, err := time.ParseDuration(tstampDeltaStr)
		if err != nil {
			f.tstampDelta = defTstampDelta
		} else {
			f.tstampDelta = tstampDelta
		}
		server.EnsureStarted(f.state, serverPort)
	})
}

type MetricTemplate struct {
	source      string
	statsSource string
	mapToStats  map[string]map[string]string
	mapToDocker map[string]map[string]string
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
	extractMapping := func(obj interface{}) map[string]map[string]string {
		const tmplMarker = "__tmpl"
		mapping := map[string]map[string]string {}
		tmplWalker := util.NewObjWalker(obj)
		tmplWalker.Walk("/", func(target string, info os.FileInfo, _ error) error {
			var spec map[string]interface{}
			var isMap bool
			pureMap := true
			if spec, isMap = info.Sys().(map[string]interface{}); !isMap {
				pureMap = false
				spec, isMap = util.ExtractCompactValueSpec(info.Sys())
			}
			if isMap {
				if _, isSpec := spec[tmplMarker]; !isSpec {
					return nil
				}
				spec["target"] = target
				valueSpec := map[string]string {}
				for k, v := range spec {
					valueSpec[k] = v.(string)
				}
				src := valueSpec["src"]
				// if have another spec for this source path,
				//record new value spec as alias
				if rootMapping, haveRoot := mapping[src]; haveRoot {
					alias := uuid.NewV1().String()
					aliases, haveAliases := rootMapping["aliases"]
					if !haveAliases {
						aliases = alias
					} else {
						aliases = aliases + ":" + alias
					}
					rootMapping["aliases"] = aliases
					mapping[alias] = valueSpec
				} else {
					mapping[src] = valueSpec
				}
				if pureMap {
					return filepath.SkipDir
				}
			}
			return nil
		})
		return mapping
	}
	// replace value specs in template object with default values that those specs provide
	applyDefaults := func(obj interface{}, mapping map[string]map[string]string) {
		w := util.NewObjWalker(obj)
		vp := util.NewValueProvider()
		for _, spec := range mapping {
			node, _ := w.Seek(filepath.Dir(spec["target"]))
			nodeAsMap := node.(map[string]interface{})
			leafName := filepath.Base(spec["target"])
			defVal, gotDefault := vp.GetDefaultOr(spec)
			if gotDefault {
				nodeAsMap[leafName] = defVal
			} else {
				delete(nodeAsMap, leafName)
			}
		}
	}
	pri := func(pfx string, val interface{}) {
		//valb, _ := json.MarshalIndent(val, "", "  ")
		//fmt.Printf("%s) %#s\n", pfx, valb)
	}
	statsListRef, _ := util.NewObjWalker(templateObj).Seek("/stats")
	statsList := statsListRef.([]interface{})
	var statsObj interface{}
	statsObj, statsList = statsList[0], statsList[1:]
	map[string]interface{}(templateObj)["stats"] = statsList
	// extract template mappings
	////FIXME:REMOVEIT\/
	pri("\n\n\nthe statsObj", statsObj)
	pri("\nthe templateObj", templateObj)
	mapToStats :=  extractMapping(statsObj)
	mapToDocker := extractMapping(templateObj)
	////FIXME:REMOVEIT\/
	pri("\n\n\nthe mapToStats", mapToStats)
	pri("\nthe mapToDocker", mapToDocker)
	// replace the template positions with default values
	applyDefaults(statsObj, mapToStats)
	applyDefaults(templateObj, mapToDocker)
	////FIXME:REMOVEIT\/
	pri("\n\n\nthe statsObj-1", statsObj)
	pri("\nthe templateObj-1", templateObj)
	statsTemplate, _ := json.Marshal(statsObj)
	dockerTemplate, _ := json.Marshal(templateObj)
	f.metricTemplate = MetricTemplate{
		source:      string(dockerTemplate),
		statsSource: string(statsTemplate),
		mapToStats:  mapToStats,
		mapToDocker: mapToDocker,
	}
	return nil
}

func (f *core) LoadMetricTemplate(path string) {
	f.exportTmplFile = path
	if err := f.loadMetricTemplate(); err != nil {
		panic(err)
	}
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
	stats_dockersPcsdMap := map[string]bool{}
	stats_statsPcsdMap := map[string]bool{}
	//-- worker functions handling different parts of the metrics
	fetchObjectForDocker := func(id, path string, metric *plugin.MetricType) (obj map[string]interface{}, existedBefore bool) {
		//TODO: support the docker tree
		stats_dockersPcsdMap[path] = true
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
			tstamp := metric.Timestamp().Add(f.tstampDelta)
			statsObj["timestamp"] = tstamp.Format("2006-01-02T15:04:05Z07:00")
			temporaryStats[path] = statsObj
			return statsObj, true
		} else {
			return statsObj, false
		}
	}
	validateStatsMetric := func(path, ns string) ([]string, bool) {
		for sourcePath, _ := range f.metricTemplate.mapToStats {
			if strings.HasSuffix(ns, sourcePath) {
				convSpec := f.metricTemplate.mapToStats[sourcePath]
				if aliases, haveAliases := convSpec["aliases"]; haveAliases {
					sourcePaths := append(strings.Split(aliases, ":"), sourcePath)
					return sourcePaths, true
				}
				return []string{sourcePath}, true
			}
		}
		customPath := ns[strings.LastIndex(ns, path)+len(path):]
		return []string{customPath}, false
	}
	validateDockerMetric := func(path, ns string) ([]string, bool) {
		for sourcePath, _ := range f.metricTemplate.mapToDocker {
			if strings.HasSuffix(ns, sourcePath) {
				convSpec := f.metricTemplate.mapToDocker[sourcePath]
				if aliases, haveAliases := convSpec["aliases"]; haveAliases {
					sourcePaths := append(strings.Split(aliases, ":"), sourcePath)
					return sourcePaths, true
				}
				return []string{sourcePath}, true
			}
		}
		return []string{}, false
	}
	insertIntoStats := func(dockerPath string, statsObj map[string]interface{}, metric *plugin.MetricType) (didInsert bool) {
		ns := metric.Namespace().String()
		didInsert = false
		if sourcePaths, isStatsMetric := validateStatsMetric(dockerPath, ns); isStatsMetric {
			for _, sourcePath := range sourcePaths {
				targetPath := f.metricTemplate.mapToStats[sourcePath]["target"]
				metricParent, _ := util.NewObjWalker(statsObj).Seek(filepath.Dir(targetPath))
				metricParentMap := metricParent.(map[string]interface{})
				metricParentMap[filepath.Base(targetPath)] = metric.Data()
				didInsert = true
			}
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
		sourcePaths, isDockerMetric := validateDockerMetric(dockerPath, ns)
		if !isDockerMetric {
			return
		}
		for _, sourcePath := range sourcePaths {
			targetPath := f.metricTemplate.mapToDocker[sourcePath]["target"]
			metricParent, _ := util.NewObjWalker(dockerObj).Seek(filepath.Dir(targetPath))
			metricParentMap := metricParent.(map[string]interface{})
			metricParentMap[filepath.Base(targetPath)] = metric.Data()
			didInsert = true
		}
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
	//-- main processing loop
	firstTimeDockers := map[string]bool{}
	for _, mt := range metrics {
		if id, path, isDockerMetric := f.extractDockerIdAndPath(&mt); isDockerMetric {
			dockerObj, knownDocker := fetchObjectForDocker(id, path, &mt)
			if !knownDocker {
				firstTimeDockers[path] = true
			}
			statsObj, _ := fetchObjectForStats(id, path, &mt)
			if insertIntoStats(path, statsObj, &mt) {
				stats_statsPcsdMap[path] = true
				continue
			}
			if _, firstTimeDocker := firstTimeDockers[path]; firstTimeDocker {
				insertIntoDocker(path, dockerObj, &mt)
			}
		}
	}
	for path, id := range dockerPaths {
		mergeStatsForDocker(id, path)
	}
	// update core stats for debugging
	f.stats.metricsRxRecently = len(metrics)
	f.stats.metricsRxTotal += len(metrics)
	if len(stats_dockersPcsdMap) > f.stats.containersRxMax {
		f.stats.containersRxMax = len(stats_dockersPcsdMap)
	}
	f.stats.containersRxRecently = len(stats_dockersPcsdMap)
	stats_statsPcsdNum := len(stats_statsPcsdMap)
	if stats_statsPcsdNum > f.stats.statsRxMax {
		f.stats.statsRxMax = stats_statsPcsdNum
	}
	f.stats.statsRxRecently = stats_statsPcsdNum
	f.stats.statsRxTotal += stats_statsPcsdNum
	f.logger.Infof("processing stats: %+v\n", f.stats)
}
