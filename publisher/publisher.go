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

	"github.com/intelsdi-x/kubesnap-plugin-publisher-heapster/exchange"
	"github.com/intelsdi-x/kubesnap-plugin-publisher-heapster/server"
	"github.com/intelsdi-x/snap/control/plugin"
	"github.com/intelsdi-x/snap/control/plugin/cpolicy"
	"github.com/intelsdi-x/snap/core/ctypes"
	cadv "github.com/google/cadvisor/info/v1"
	"os"
	"sync"
	"time"
	"runtime/debug"
	"strings"
)

const (
	name       = "heapster"
	version    = 1
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

const (
	customMetricName = "custom_metric_name"
	customMetricType = "custom_metric_type"
	customMetricFormat = "custom_metric_format"
	customMetricUnits = "custom_metric_units"
	customMetricContainerPath = "custom_metric_container_path"

	defCustomMetricType = "gauge"
	defCustomMetricFormat = "int"
	defCustomMetricUnits = "none"
	defCustomMetricContainerPath = "/"
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

func init() {
	gob.Register(map[string]float64 {})
}

func NewInnerState() *exchange.InnerState {
	res := &exchange.InnerState{
		DockerPaths:   map[string]string{},
		DockerStorage: map[string]interface{}{},
		PendingMetrics:map[string]map[string][]cadv.MetricVal {},
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
			debug.PrintStack()
			panic(r)
		}
	}()
        initErr := f.ensureInitialized(config)
        if initErr != nil {
             f.logger.Printf("Server not initialized, error=%v\n", initErr)
        }
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
                plugin.Exclusive(true))
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

func (f *core) ensureInitialized(config map[string]ctypes.ConfigValue) error {
	configMap := ConfigMap(config)
        var serr error
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
		serr = server.EnsureStarted(f.state, serverPort)
	})
        return serr
}

func init() {
	//if os.Getenv("DISABLE_PRI") == "1" {
	if os.Getenv("ENABLE_PRI") != "1" {
		pri = func(_ string, _ ...interface{}) {
			// nop
		}
	}
}

var pri = func(format string, item ...interface{}) {
	if strings.Contains(format, "%") || len(item) == 0 {
		fmt.Fprintf(os.Stderr, format +"\n", item...)
	} else {
		fmt.Fprintf(os.Stderr, format +"%v \n", item...)
	}
}
