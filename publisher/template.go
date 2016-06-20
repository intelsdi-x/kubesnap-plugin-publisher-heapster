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
	"strings"
	"github.com/intelsdi-x/kubesnap-plugin-publisher-heapster/util"
	"os"
	"github.com/satori/go.uuid"
	"path/filepath"
	"io/ioutil"
)

type MetricTemplate struct {
	source      string
	statsSource string
	ifaceSource string
	fsSource string
	mapToStats  map[string]map[string]string
	mapToDocker map[string]map[string]string
	mapToIface  map[string]map[string]string
	mapToFs map[string]map[string]string
}

func (f *core) loadMetricTemplate() error {
	var err error
	var source string
	if source, err = f.loadTemplateSource(); err != nil {
		return err
	}
	var templateRef interface{}
	// parse template once for test
	decoder := json.NewDecoder(strings.NewReader(source))
	decoder.UseNumber()

	//if err = json.Unmarshal([]byte(source), &templateRef); err != nil {
	if err = decoder.Decode(&templateRef); err != nil {
		return err
	}
	templateObj := templateRef.(map[string]interface{})
	extractMapping := func(obj interface{}) map[string]map[string]string {
		const tmplMarker = "__tmpl"
		mapping := map[string]map[string]string{}
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
				valueSpec := map[string]string{}
				for k, v := range spec {
					valueSpec[k] = v.(string)
				}
				src := valueSpec["src"]
				if ellIdx := strings.LastIndex(src, "..."); ellIdx >= 0 {
					ptrn := ""
					ptrn, src = src[:ellIdx], src[ellIdx + 3:]
					ptrn = strings.Replace(ptrn, "...", ".*", -1)
					valueSpec["src"] = src
					valueSpec["ptrn"] = ptrn
				}
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
	//pri := func(pfx string, val interface{}) {
	//	//valb, _ := json.MarshalIndent(val, "", "  ")
	//	//fmt.Printf("%s) %#s\n", pfx, valb)
	//}
	statsListRef, _ := util.NewObjWalker(templateObj).Seek("/stats")
	statsList := statsListRef.([]interface{})
	var statsObj interface{}
	statsObj, statsList = statsList[0], statsList[1:]
	map[string]interface{}(templateObj)["stats"] = statsList

	ifaceListRef, _ := util.NewObjWalker(statsObj).Seek("/network/interfaces")
	ifaceList := ifaceListRef.([]interface{})
	var ifaceObj interface{}
	ifaceObj, ifaceList = ifaceList[0], ifaceList[1:]
	networkRef, _ := util.NewObjWalker(statsObj).Seek("/network")
	network := networkRef.(map[string]interface{})
	network["interfaces"] = map[string]interface{}{}

	// convert filesystem list to a map
	fsListRef, _ := util.NewObjWalker(statsObj).Seek("/filesystem")
	fsList := fsListRef.([]interface{})
	var fsObj interface{}
	fsObj, fsList = fsList[0], fsList[1:]
	statsMap := statsObj.(map[string]interface{})
	statsMap["filesystem"] = map[string]interface{} {}

	// extract template mappings
	////FIXME:REMOVEIT
	pri("\n\n\nthe statsObj", statsObj)
	//pri("\nthe templateObj", templateObj)
	//pri("\nthe ifaceObj", ifaceObj)
	mapToStats := extractMapping(statsObj)
	mapToDocker := extractMapping(templateObj)
	mapToIface := extractMapping(ifaceObj)
	mapToFs := extractMapping(fsObj)
	////FIXME:REMOVEIT
	//pri("\n\n\nthe mapToStats", mapToStats)
	//pri("\nthe mapToDocker", mapToDocker)
	//pri("\nthe mapToDocker", mapToIface)
	pri("\nthe mapToFs", mapToFs)
	// replace the template positions with default values
	applyDefaults(statsObj, mapToStats)
	applyDefaults(templateObj, mapToDocker)
	applyDefaults(ifaceObj, mapToIface)
	applyDefaults(fsObj, mapToFs)
	////FIXME:REMOVEIT
	//pri("\n\n\nthe statsObj-1", statsObj)
	//pri("\nthe templateObj-1", templateObj)
	//pri("\nthe ifaceObj-1", ifaceObj)
	pri("\nthe fsObj-1", fsObj)
	statsTemplate, _ := json.Marshal(statsObj)
	dockerTemplate, _ := json.Marshal(templateObj)
	ifaceTemplate, _ := json.Marshal(ifaceObj)
	fsTemplate, _ := json.Marshal(fsObj)
	f.metricTemplate = MetricTemplate{
		source:      string(dockerTemplate),
		statsSource: string(statsTemplate),
		ifaceSource: string(ifaceTemplate),
		fsSource: string(fsTemplate),
		mapToStats:  mapToStats,
		mapToDocker: mapToDocker,
		mapToIface:  mapToIface,
		mapToFs: mapToFs,
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
