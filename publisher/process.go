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
	"encoding/json"
	cadv "github.com/google/cadvisor/info/v1"
	"github.com/intelsdi-x/kubesnap-plugin-publisher-heapster/util"
	"github.com/intelsdi-x/snap/control/plugin"
	score "github.com/intelsdi-x/snap/core"
	"path/filepath"
	"reflect"
	"strings"
	"time"
	"regexp"
)

type processorContext struct {
	*core
	temporaryStats       map[string]map[string]interface{}
	stats_dockersPcsdMap map[string]bool
	stats_statsPcsdMap   map[string]bool
}

func (f *core) processMetrics(metrics []plugin.MetricType) {
	ctx := &processorContext{
		core:                 f,
		temporaryStats:       map[string]map[string]interface{}{},
		stats_dockersPcsdMap: map[string]bool{},
		stats_statsPcsdMap:   map[string]bool{},
	}
	ctx.processMetrics0(metrics)
}

func (f *processorContext) processMetrics0(metrics []plugin.MetricType) {
	firstTimeDockers := map[string]bool{}
	countRegularStats := 0
	for _, mt := range metrics {
		if id, path, isDockerMetric, isCustomMetric := f.extractDockerIdAndPath(&mt); isDockerMetric {
			dockerObj, knownDocker := f.fetchObjectForDocker(id, path, &mt)
			if !knownDocker {
				firstTimeDockers[path] = true
			}
			_, firstTimeDocker := firstTimeDockers[path]
			statsObj, _ := f.fetchObjectForStats(id, path, &mt)
			if f.insertIntoStats(path, statsObj, &mt) {
				f.stats_statsPcsdMap[path] = true
				goto finish
			}
			if f.insertIntoIface(path, statsObj, &mt) {
				goto finish
			}
			if f.insertIntoFs(path, statsObj, &mt) {
				goto finish
			}
			if knownDocker && f.insertIntoCustomMetrics(path, dockerObj, &mt) {
				goto finish
			}
			if firstTimeDocker && f.insertIntoDocker(path, dockerObj, &mt) {
				goto finish
			}
		finish:
			if !isCustomMetric {
				countRegularStats++
			}

		}
	}
	if countRegularStats > 0 {
		for path, id := range f.state.DockerPaths {
			f.mergeStatsForDocker(id, path)
		}
	}

	//-- DEBUG - update core stats for debugging - completely optional part
	//FIXME:RMVIT\/
	{
		maxPendingStats := 0
		for _, m := range f.state.PendingMetrics {
			for _, v := range m {
				if len(v) > maxPendingStats {
					maxPendingStats = len(v)
				}
			}
		}
		pri("max no# pending stats: %v", maxPendingStats)
	}
	f.stats.metricsRxRecently = len(metrics)
	f.stats.metricsRxTotal += len(metrics)
	if len(f.stats_dockersPcsdMap) > f.stats.containersRxMax {
		f.stats.containersRxMax = len(f.stats_dockersPcsdMap)
	}
	f.stats.containersRxRecently = len(f.stats_dockersPcsdMap)
	stats_statsPcsdNum := len(f.stats_statsPcsdMap)
	if stats_statsPcsdNum > f.stats.statsRxMax {
		f.stats.statsRxMax = stats_statsPcsdNum
	}
	f.stats.statsRxRecently = stats_statsPcsdNum
	f.stats.statsRxTotal += stats_statsPcsdNum
	f.logger.Infof("processing stats: %+v\n", f.stats)
}


//// stats EXTRACTION methods

func (f *processorContext) validateMetricWithMap(dockerPath, ns string, mapping map[string]map[string]string) ([]string, bool) {
	for sourcePath, _ := range mapping {
		if strings.HasSuffix(ns, sourcePath) {
			sourcePaths := []string {}
			if aliases, haveAliases := mapping[sourcePath]["aliases"]; haveAliases {
				sourcePaths = append(strings.Split(aliases, ":"), sourcePath)
			} else {
				sourcePaths = []string{sourcePath}
			}
			// validate source paths as they may have any pattern ("ptrn") filters
			filtered := sourcePaths[:0]
			for _, path := range sourcePaths {
				if ptrn, havePtrn := mapping[path]["ptrn"]; havePtrn {
					if matched, err := regexp.MatchString(ptrn, ns); !matched || err != nil {
						continue
					}
				}
				filtered = append(filtered, path)
			}
			sourcePaths = filtered
			if len(sourcePaths) > 0 {
				return sourcePaths, true
			}
			return nil, false
		}
	}
	customPath := ns[strings.LastIndex(ns, dockerPath)+len(dockerPath):]
	return []string{customPath}, false

}
func (f *processorContext) validateStatsMetric(dockerPath, ns string) ([]string, bool) {
	return f.validateMetricWithMap(dockerPath, ns, f.metricTemplate.mapToStats)
}
func (f *processorContext) validateDockerMetric(dockerPath, ns string) ([]string, bool) {
	return f.validateMetricWithMap(dockerPath, ns, f.metricTemplate.mapToDocker)
}
func (f *processorContext) validateIfaceMetric(dockerPath, ns string) ([]string, bool) {
	return f.validateMetricWithMap(dockerPath, ns, f.metricTemplate.mapToIface)
}
func (f *processorContext) validateFsMetric(dockerPath, ns string) ([]string, bool) {
	return f.validateMetricWithMap(dockerPath, ns, f.metricTemplate.mapToFs)
}

func (f *processorContext) validateCustomMetric(metric *plugin.MetricType) (spec cadv.MetricSpec, validMetric bool) {
	if _, spec, validMetric = f.extractOneCustomMetric(metric); validMetric {
		return spec, true
	}
	return spec, false
}

func (f *processorContext) extractIfaceMetric(metric *plugin.MetricType) (string, string) {
	nsSplit := metric.Namespace().Strings()
	// /intel/docker/DOCKER_ID/network/IFACE_ID/METRIC
	lens := len(nsSplit)
	return nsSplit[lens-2], nsSplit[lens-1]
}

func (f *processorContext) extractFsMetric(metric *plugin.MetricType) (string, string) {
	nsSplit := metric.Namespace().Strings()
	// /intel/docker/DOCKER_ID/filesystem/FS_ID/METRIC
	lens := len(nsSplit)
	return nsSplit[lens-2], nsSplit[lens-1]
}

func (f *processorContext) fetchObjectForDocker(id, path string, metric *plugin.MetricType) (obj map[string]interface{}, existedBefore bool) {
	f.stats_dockersPcsdMap[path] = true
	if dockerObj, gotIt := f.state.DockerStorage[path]; gotIt {
		dockerMap := dockerObj.(map[string]interface{})
		return dockerMap, true
	} else {
		f.state.DockerPaths[path] = id
		var dockerMap map[string]interface{}
		json.Unmarshal([]byte(f.metricTemplate.source), &dockerMap)
		dockerMap["id"] = id
		dockerMap["name"] = path
		if id == "root" {
			dockerMap["id"] = "/"
			dockerMap["name"] = "/"
		}

		f.state.DockerStorage[path] = dockerMap
		return dockerMap, false
	}
}
// fetchObjectForStats gets an allocated stats object for storing
//metrics; no object will be allocated if metric argument is  nil
func (f *processorContext) fetchObjectForStats(id, path string, metric *plugin.MetricType) (map[string]interface{}, bool) {
	var statsObj map[string]interface{}
	var haveStats bool
	if statsObj, haveStats = f.temporaryStats[path]; haveStats {
		return statsObj, true
	} else if metric != nil {
		json.Unmarshal([]byte(f.metricTemplate.statsSource), &statsObj)
		tstamp := metric.Timestamp().Add(f.tstampDelta)
		statsObj["timestamp"] = tstamp.Format("2006-01-02T15:04:05Z07:00")
		f.temporaryStats[path] = statsObj
		return statsObj, true
	} else {
		return statsObj, false
	}
}

func (f *processorContext) fetchObjectForIface(statsMap map[string]interface{}, metric *plugin.MetricType) (map[string]interface{}, bool) {
	ifacesMapRef, _ := util.NewObjWalker(statsMap).Seek("/network/interfaces")
	ifacesMap := ifacesMapRef.(map[string]interface{})
	ifaceName, _ := f.extractIfaceMetric(metric)
	if iface, haveIface := ifacesMap[ifaceName]; haveIface {
		return iface.(map[string]interface{}), true
	} else {
		var ifaceObj map[string]interface{}
		json.Unmarshal([]byte(f.metricTemplate.ifaceSource), &ifaceObj)
		ifacesMap[ifaceName] = ifaceObj
		return ifaceObj, true
	}

}

func (f *processorContext) fetchObjectForFs(statsMap map[string]interface{}, metric *plugin.MetricType) (map[string]interface{}, bool) {
	fsMapRef, _ := util.NewObjWalker(statsMap).Seek("/filesystem")
	fsMap := fsMapRef.(map[string]interface{})
	fsName, _ := f.extractFsMetric(metric)
	if fs, haveFs := fsMap[fsName]; haveFs {
		return fs.(map[string]interface{}), true
	} else {
		var fsObj map[string]interface{}
		json.Unmarshal([]byte(f.metricTemplate.fsSource), &fsObj)
		fsMap[fsName] = fsObj
		return fsObj, true
	}

}

func (f *processorContext) extractCustomMetrics(metric *plugin.MetricType) (dockerPath string, specs []cadv.MetricSpec, valid bool) {
	copyMetric := func(metric plugin.MetricType, nsSuffix string) *plugin.MetricType {
		res := metric
		res.Tags_ = make(map[string]string, len(metric.Tags_))
		for k, v := range metric.Tags_ {
			res.Tags_[k] = v
		}
		res.Namespace_ = metric.Namespace_[:]
		if nsSuffix != "" {
			res.Namespace_ = score.Namespace(res.Namespace_).AddStaticElement(nsSuffix)
		}
		return &res
	}
	if valueMap, isMap := metric.Data().(map[string]float64); !isMap {
		dockerPath1, spec1, valid1 := f.extractOneCustomMetric(metric)
		if !valid1 {
			return "", specs, false
		}
		specs = []cadv.MetricSpec {spec1}
		return dockerPath1, specs, true
	} else {
		specs = make([]cadv.MetricSpec, 0, len(valueMap))
		valid = false
		dockerPath = ""
		for k, v := range valueMap {
			nuMetric := copyMetric(*metric, k)
			nuMetric.Data_ = v
			dockerPath1, spec1, valid1 := f.extractOneCustomMetric(nuMetric)
			if dockerPath == "" {
				dockerPath = dockerPath1
			}
			valid = valid || valid1
			specs = append(specs, spec1)
		}
		if valid {
			return dockerPath, specs, true
		}
		return "", specs[:0], false
	}
}

func (f *processorContext) extractOneCustomMetric(metric *plugin.MetricType) (dockerPath string, spec cadv.MetricSpec, valid bool) {
	tags := metric.Tags()
	ns := metric.Namespace()
	dockerPath = ""
	spec = cadv.MetricSpec{
		Type:   defCustomMetricType,
		Format: defCustomMetricFormat,
	}
	var haveName, haveType, haveFormat, haveUnits, haveDockerPath bool
	if spec.Name, haveName = tags[customMetricName]; !haveName {
		spec.Name = strings.Join(ns.Strings(), "/")
	}
	tmpTag := ""
	if tmpTag, haveType = tags[customMetricType]; haveType {
		spec.Type = cadv.MetricType(tmpTag)
	}
	if tmpTag, haveFormat = tags[customMetricFormat]; haveFormat {
		spec.Format = cadv.DataType(tmpTag)
	}
	if spec.Units, haveUnits = tags[customMetricUnits]; !haveUnits {
		spec.Units = defCustomMetricUnits
	}
	if dockerPath, haveDockerPath = tags[customMetricContainerPath]; !haveDockerPath {
		dockerPath = defCustomMetricContainerPath
	}
	if haveName || haveType || haveFormat || haveUnits || haveDockerPath {
		return dockerPath, spec, true
	} else {
		return "", spec, false
	}
}

func (f *processorContext) extractCustomValues(metric *plugin.MetricType, specs []cadv.MetricSpec) map[string]cadv.MetricVal {
	res := make(map[string]cadv.MetricVal, len(specs))
	if valueMap, isMap := metric.Data_.(map[string]float64); !isMap {
		//FIXME:RMVIT\/
		pri("custom_metrics: now probing value for %v, of %v; got specs: %+v \n", metric.Namespace().String(), reflect.TypeOf(metric.Data()), specs)
		value, ok := f.extractOneCustomValue(&specs[0], metric.Timestamp_, metric.Data_)
		if ok {
			res[specs[0].Name] = value
		}
	} else {
		for _, spec := range specs {
			value, ok := f.extractOneCustomValue(&spec, metric.Timestamp_, valueMap[filepath.Base(spec.Name)])
			//FIXME:RMVIT\/
			pri("custom_metrics: probing one of values for %v, key: %v, value of: %v; got spec: %+v; is it ok: %v \n", metric.Namespace().String(), spec.Name, reflect.TypeOf(valueMap[spec.Name]), spec, ok)
			if ok {
				res[spec.Name] = value
			}
		}
	}
	return res
}

func (f *processorContext) extractOneCustomValue(spec *cadv.MetricSpec, timeStamp time.Time, value interface{}) (cadv.MetricVal, bool) {
	customVal := cadv.MetricVal{Timestamp: timeStamp}
	switch spec.Format {
	case cadv.IntType:
		switch i := value.(type) {
		case int64:
			customVal.IntValue = int64(i)
		case uint64:
			customVal.IntValue = int64(i)
		case int32:
			customVal.IntValue = int64(i)
		case uint32:
			customVal.IntValue = int64(i)
		case uint16:
			customVal.IntValue = int64(i)
		case int16:
			customVal.IntValue = int64(i)
		case uint8:
			customVal.IntValue = int64(i)
		case int8:
			customVal.IntValue = int64(i)
		case uint:
			customVal.IntValue = int64(i)
		case int:
			customVal.IntValue = int64(i)
		default:
			pri("metric %s cant be handled as IntValue", spec.Name)
			return customVal, false
		}
	case cadv.FloatType:
		switch i := value.(type) {
		case float32:
			customVal.FloatValue = float64(i)
		case float64:
			customVal.FloatValue = float64(i)
		case int64:
			customVal.FloatValue = float64(i)
		case uint64:
			customVal.FloatValue = float64(i)
		case int32:
			customVal.FloatValue = float64(i)
		case uint32:
			customVal.FloatValue = float64(i)
		case int16:
			customVal.FloatValue = float64(i)
		case uint16:
			customVal.FloatValue = float64(i)
		case int8:
			customVal.FloatValue = float64(i)
		case uint8:
			customVal.FloatValue = float64(i)
		case int:
			customVal.FloatValue = float64(i)
		case uint:
			customVal.FloatValue = float64(i)
		default:
			pri("metric %s cant be handled as FloatValue", spec.Name)
			return customVal, false
		}
	}
	return customVal, true
}

func (f *processorContext) extractDockerIdAndPathForCustomMetric(metric *plugin.MetricType) (string, string, bool) {
	if dockerPath, _, valid := f.extractCustomMetrics(metric); !valid {
		return "", "", false
	} else {
		id := filepath.Base(dockerPath)
		return id, dockerPath, true
	}

}

func (f *processorContext) extractDockerIdAndPath(metric *plugin.MetricType) (id string, path string, anyMetric bool, customMetric bool) {
	ns := metric.Namespace().String()
	if strings.HasPrefix(ns, dockerMetricPrefix) {
		tailSplit := strings.Split(strings.TrimLeft(strings.TrimPrefix(ns, dockerMetricPrefix), "/"), "/")
		id := tailSplit[0]
		path := "/" + id
		if id == "root" {
			id = "/"
			path = "/"
		}
		return id, path, true, false
	} else if id, path, validCustomMetric := f.extractDockerIdAndPathForCustomMetric(metric); validCustomMetric {
		return id, path, true, true
	} else {
		return "", "", false, false
	}
}


//// INSERTING statistics into publisher's state

func (f *processorContext) insertIntoStats(dockerPath string, statsObj map[string]interface{}, metric *plugin.MetricType) (didInsert bool) {
	ns := metric.Namespace().String()
	didInsert = false
	if sourcePaths, isStatsMetric := f.validateStatsMetric(dockerPath, ns); isStatsMetric {
		for _, sourcePath := range sourcePaths {
			targetPath := f.metricTemplate.mapToStats[sourcePath]["target"]
			metricParent, _ := util.NewObjWalker(statsObj).Seek(filepath.Dir(targetPath))
			metricParentMap := metricParent.(map[string]interface{})
			metricParentMap[filepath.Base(targetPath)] = metric.Data()
			didInsert = true
		}
	}
	return
}
func (f *processorContext) insertIntoIface(dockerPath string, statsObj map[string]interface{}, metric *plugin.MetricType) (didInsert bool) {
	ns := metric.Namespace().String()
	if sourcePaths, isIfaceMetric := f.validateIfaceMetric(dockerPath, ns); !isIfaceMetric {
		return false
	} else {
		ifaceObj, _ := f.fetchObjectForIface(statsObj, metric)
		for _, sourcePath := range sourcePaths {
			targetPath := f.metricTemplate.mapToIface[sourcePath]["target"]
			metricParent, _ := util.NewObjWalker(ifaceObj).Seek(filepath.Dir(targetPath))
			metricParentMap := metricParent.(map[string]interface{})
			metricParentMap[filepath.Base(targetPath)] = metric.Data()
			didInsert = true
		}
		return true
	}
}

func (f *processorContext) insertIntoFs(dockerPath string, statsObj map[string]interface{}, metric *plugin.MetricType) (didInsert bool) {
	ns := metric.Namespace().String()
	if sourcePaths, isFsMetric := f.validateFsMetric(dockerPath, ns); !isFsMetric {
		return false
	} else {
		fsObj, _ := f.fetchObjectForFs(statsObj, metric)
		for _, sourcePath := range sourcePaths {
			targetPath := f.metricTemplate.mapToFs[sourcePath]["target"]
			metricParent, _ := util.NewObjWalker(fsObj).Seek(filepath.Dir(targetPath))
			metricParentMap := metricParent.(map[string]interface{})
			metricParentMap[filepath.Base(targetPath)] = metric.Data()
			didInsert = true
		}
		return true
	}
}

func (f *processorContext) insertIntoDocker(dockerPath string, dockerObj map[string]interface{}, metric *plugin.MetricType) (didInsert bool) {
	ns := metric.Namespace().String()
	didInsert = false
	sourcePaths, isDockerMetric := f.validateDockerMetric(dockerPath, ns)
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

func (f *processorContext) insertIntoCustomMetrics(dockerPath string, dockerObj map[string]interface{}, metric *plugin.MetricType) (didInsert bool) {
	didInsert = false
	_, specs, valid := f.extractCustomMetrics(metric)
	if !valid {
		return
	}
	values := f.extractCustomValues(metric, specs)
	//-- insert spec
	specMap := dockerObj["spec"].(map[string]interface{})
	metricList := specMap["custom_metrics"].([]interface{})
	dbg_valuesIn := []string {}
	for _, spec := range specs {
		foundSpec := false
		for _, ckSpecObj := range metricList {
			ckSpec := ckSpecObj.(cadv.MetricSpec)
			if ckSpec.Name == spec.Name {
				foundSpec = true
				break
			}
		}
		if !foundSpec {
			metricList = append(metricList, spec)
			specMap["custom_metrics"] = metricList
		}
		customVal, validVal := values[spec.Name]
		if !validVal {
			//FIXME:RMVIT\/
			pri("custom_val/insert: tried for %v, found: false", spec.Name)
			continue
		}
		//FIXME:RMVIT\/
		pri("custom_val/insert: tried for %v, found: true, got: %+v", spec.Name, customVal)
		dbg_valuesIn = append(dbg_valuesIn, spec.Name)

		// find room for custom metrics
		dockerValuesMap, gotDockerValuesMap := f.state.PendingMetrics[dockerPath]
		if !gotDockerValuesMap {
			dockerValuesMap = map[string][]cadv.MetricVal{}
			f.state.PendingMetrics[dockerPath] = dockerValuesMap
		}
		statsList, _ := dockerValuesMap[spec.Name]
		statsList = append(statsList, customVal)
		dockerValuesMap[spec.Name] = statsList
		didInsert = didInsert || true
	}
	//FIXME:RMVIT\/
	defer pri("custom_metrics: did we insert?: %v, resulting values: %v\n", didInsert, dbg_valuesIn)
	return
}


//// MERGING stats from temporary structures into  stats element for container

func (f *processorContext) mergeStatsForDocker(id, path string) {
	dockerObj, _ := f.fetchObjectForDocker(id, path, nil)
	statsObj, haveStats := f.fetchObjectForStats(id, path, nil)
	if !haveStats || len(statsObj) == 0 {
		// no stats for that docker were allocated in this round of processing
		return
	}
	// convert iface map to iface list, as expected by consumers of the REST API
	networkRef, _ := util.NewObjWalker(statsObj).Seek("/network")
	ifaceMapRef, _ := util.NewObjWalker(networkRef).Seek("/interfaces")
	ifaceMap := ifaceMapRef.(map[string]interface{})
	networkMap := networkRef.(map[string]interface{})
	ifaceList := []interface{}{}
	for _, ifaceObj := range ifaceMap {
		ifaceList = append(ifaceList, ifaceObj)
	}
	networkMap["interfaces"] = ifaceList

	// convert fs map to fs list, as expected by consumers
	fsMapRef, _ := util.NewObjWalker(statsObj).Seek("/filesystem")
	fsMap := fsMapRef.(map[string]interface{})
	fsList := []interface{} {}
	for _, fsObj := range fsMap {
		fsList = append(fsList, fsObj)
	}
	statsObj["filesystem"] = fsList

	// add in-progress stats element to statsList
	statsList := dockerObj["stats"].([]interface{})
	f.makeRoomForStats(&statsList, statsObj)
	statsList = append(statsList, statsObj)
	dockerObj["stats"] = statsList

	// merge custom metrics
	f.mergePendingMetrics(path, statsList)
	f.dropTooOldPendingMetrics(path, statsList)
}

// make sure we don't overflow  statsDepth nor  statsSpan when
//new  statsObj is added
func (f *processorContext) makeRoomForStats(destStatsList *[]interface{}, statsObj map[string]interface{}) {
	validOfs := 0
	statsList := *destStatsList
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
	destStatsList = &statsList
}

func (f *processorContext) mergePendingMetrics(path string, statsList []interface{}) {
	dockerValuesMap, gotDockerValuesMap := f.state.PendingMetrics[path]
	var statsObj map[string]interface{}
	if !gotDockerValuesMap {
		return
	}
	for _, statsElem := range statsList {
		statsObj = statsElem.(map[string]interface{})
		targetMap := statsObj["custom_metrics"].(map[string]interface{})
		refStamp, _ := util.ParseTime(statsObj["timestamp"].(string))
		for metricName, valueList := range dockerValuesMap {
			filteredPendingValues := make([]cadv.MetricVal, 0, len(valueList))
			for _, value := range valueList {
				if !value.Timestamp.Before(refStamp) {
					filteredPendingValues = append(filteredPendingValues, value)
					continue
				}
				targetList, _ := targetMap[metricName].([]interface{})
				targetMap[metricName] = append(targetList, value)
			}
			dockerValuesMap[metricName] = filteredPendingValues
		}
	}
}

func (f *processorContext) dropTooOldPendingMetrics(path string, statsList []interface{}) {
	dockerValuesMap, gotDockerValuesMap := f.state.PendingMetrics[path]
	var statsObj map[string]interface{}
	if !gotDockerValuesMap {
		return
	}
	var oldestStamp *time.Time = nil
	for _, statsElem := range statsList {
		statsObj = statsElem.(map[string]interface{})
		refStamp, _ := util.ParseTime(statsObj["timestamp"].(string))
		if oldestStamp == nil || refStamp.Before(*oldestStamp) {
			oldestStamp = &refStamp
		}
	}
	if oldestStamp == nil {
		return
	}
	for metricName, valueList := range dockerValuesMap {
		for i, value := range valueList {
			if value.Timestamp.Before(*oldestStamp) {
				valueList = append(valueList[:i], valueList[i+1:]...)
			}
		}
		dockerValuesMap[metricName] = valueList
	}
}
