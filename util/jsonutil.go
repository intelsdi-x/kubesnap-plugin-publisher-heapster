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

package util

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"time"
	"fmt"
	"strings"
)

type JsonWalker struct {
	fs interface{}
}

type ValueParser func(string, string) (interface{}, error)
type ValueFormatter func(interface{}, string) string

type ValueProvider struct {
	parsers map[string]ValueParser
	formatters map[string]ValueFormatter
	defaults map[string]string
}

type dummyFileInfo struct {
name  string
isDir bool
sys   interface{}
}

var NotFound = errors.New("Path not found")

func NewValueProvider() *ValueProvider {
	provider := ValueProvider{
		formatters: map[string]ValueFormatter {},
		parsers: map[string]ValueParser {},
		defaults: map[string]string {},
	}
	provider.parsers["int"] = func(text string, _ string) (interface{}, error) {
		if val, err := strconv.Atoi(text); err != nil {
			return nil, err
		} else {
			return val, nil
		}
	}
	provider.parsers["bool"] = func(text string, _ string) (interface{}, error) {
		if val, err := strconv.ParseBool(text); err != nil {
			return nil, err
		} else {
			return val, nil
		}
	}
	provider.parsers["float64"] = func(text string, _ string) (interface{}, error) {
		if val, err := strconv.ParseFloat(text, 64); err != nil {
			return nil, err
		} else {
			return val, err
		}
	}
	provider.parsers["str"] = func(text string, _ string) (interface{}, error) {
		return text, nil
	}
	defaultTimeFmt := "2006-01-02T15:04:05Z07:00"
	provider.parsers["ktime"] = func(text string, _ string) (interface{}, error) {
		if val, err := time.Parse(defaultTimeFmt, text); err != nil {
			return nil, err
		} else {
			return val, nil
		}
	}
	provider.parsers["time"] = func(text string, format string) (interface{}, error) {
		if val, err := time.Parse(format, text); err != nil {
			return nil, err
		} else {
			return val, nil
		}
	}
	provider.formatters["int"] = func(val interface{}, _ string) string {
		str := strconv.Itoa(val.(int))
		return str
	}
	provider.formatters["bool"] = func(val interface{}, _ string) string {
		str := strconv.FormatBool(val.(bool))
		return str
	}
	provider.formatters["ktime"] = func(val interface{}, _ string) string {
		str := val.(time.Time).Format(defaultTimeFmt)
		return str
	}
	provider.formatters["time"] = func(val interface{}, format string) string {
		str := val.(time.Time).Format(format)
		return str
	}
	provider.formatters["float64"] = func(val interface{}, _ string) string {
		str := strconv.FormatFloat(val.(float64), 'E', -1, 64)
		return str
	}
	provider.formatters["any"] = func(val interface{}, _ string) string {
		str := fmt.Sprintf("%v", val)
		return str
	}
	provider.defaults["str"] = ""
	provider.defaults["int"] = "0"
	provider.defaults["bool"] = "false"
	provider.defaults["ktime"] = "2009-11-10T23:00:00Z"
	provider.defaults["time"] = "2009-11-10 23:00:00 +0000 UTC"
	provider.defaults["float64"] = "0.0"
	return &provider
}

func ExtractCompactValueSpec(str interface{}) (map[string]interface{}, bool) {
	var spec map[string]interface{}
	stri, isStr := str.(string)
	if !isStr {
		return spec, false
	}
	if strings.HasPrefix(stri, "__") {
		elements := strings.Split(stri, "|")
		spec = map[string]interface{} {}
		el := len(elements)
		if el > 0 {
			spec[elements[0]]=""
		}
		if el > 1 {
			spec["src"] = elements[1]
		} else {
			return spec, false
		}
		if el > 2 {
			spec["default"] = elements[2]
		}
		if el > 3 {
			spec["type"] = elements[3]
		}
		if el > 4 {
			options := strings.TrimSpace(elements[4])
			for _, kv := range strings.Split(options, ",") {
				kvs := strings.SplitN(strings.TrimSpace(kv), "=", 2)
				spec[strings.TrimSpace(kvs[0])] = strings.TrimSpace(kvs[1])
 			}
		}
		return spec, true
	}
	return spec, false
}

func (v *ValueProvider) Convert(input interface{}, spec map[string]string, orDefault bool) (interface{}, error) {
	t := spec["type"]
	s, gotS := spec["stype"]
	if !gotS {
		s = t
	}
	tf, gotTF := spec["fmt"]
	if !gotTF {
		tf = ""
	}
	sf, gotSF := spec["sfmt"]
	if !gotSF {
		sf = ""
	}
	i, gotI := spec["itype"] // internal type
	var res interface{}
	var val interface{}
	var err error
	if s == t { // same types
		if !gotI {
			res = input
			return res, nil
		}
		// format conversion
		if s != "str" {
			return nil, fmt.Errorf("Intermediate type supported only for str:str format conversion")
		}
		if tool, gotTool := v.parsers[i]; !gotTool {
			return nil, fmt.Errorf("Got no parser for intermediate type '%s'", i)
		} else {
			if val, err = tool(input.(string), sf); err != nil {
				if orDefault {
					return v.GetDefault(spec), nil
				}
				return nil, fmt.Errorf("Failed to parse text as intermediate type '%s': %v", i, err)
			}
		}
		if tool, gotTool := v.formatters[i]; !gotTool {
			return nil, fmt.Errorf("Got no formatter for intermediate type '%s'", i)
		} else {
			res = tool(val, tf)
		}
		return res, nil
	}
	// differing types
	if s == "str" {
		if tool, gotTool := v.parsers[t]; !gotTool {
			return nil, fmt.Errorf("Got no parser for type '%s'", t)
		} else {
			if res, err = tool(input.(string), tf); err != nil {
				if orDefault {
					return v.GetDefault(spec), nil
				}
				return nil, fmt.Errorf("Failed to parse text as type '%s': %v", t, err)
			}
		}
		return res, nil
	}
	if t == "str" {
		if tool, gotTool := v.formatters[s]; !gotTool {
			return nil, fmt.Errorf("Got no formatter for type '%s'", s)
		} else {
			res = tool(input, tf)
		}
		return res, nil
	}
	// between two types
	if tool, gotTool := v.formatters[s]; !gotTool {
		return nil, fmt.Errorf("Got no formatter for type '%s'", s)
	} else {
		val = tool(input, sf)
	}
	if tool, gotTool := v.parsers[t]; !gotTool {
		return nil, fmt.Errorf("Got no parser for type '%s'", t)
	} else {
		if res, err = tool(val.(string), tf); err != nil {
			if orDefault {
				return v.GetDefault(spec), nil
			}
			return nil, fmt.Errorf("Failed to parse text as type '%s': %v", t, err)
		}
	}
	return res, nil
/*
str -> TT,TF,qwe // string to type 'qwe'
	t = TT::Parse(s,TF)
	 def = TT:Parse(d,TF)
qwe -> TT,TF,str // TT='str' // type 'qwe' to string
	t = TT:Format(s,TF)
	def = d // just take the default
qwe,ST,SF -> zxc,TT,TF // between 2 types: qwe and zxc
	t = ST:Format(s,SF) -> TT:Parse(<, TF)
	def = TT:Parse(d,TF)
str,SF ->IT<- str,TF // reformat
	t = IT:Parse(s,SF) -> IT:Format(<, TF)
	def = d // just take the default string
*/
}

func (s *ValueProvider) GetDefault(spec map[string]string) interface{} {
	defVal, _ := s.GetDefaultOr(spec)
	return defVal
}

func (s *ValueProvider) GetDefaultOr(spec map[string]string) (interface{}, bool) {
	t := spec["type"]
	_, noDefault := spec["no_default"]
	d, gotD := spec["default"]
	if !gotD || noDefault {
		d = s.defaults[t]
	}
	f, gotF := spec["fmt"]
	if !gotF {
		f = ""
	}
	if tool, gotTool := s.parsers[t]; !gotTool {
		panic(fmt.Errorf("Have no parser for type '%s' to provide the default", t))
	} else {
		val, err := tool(d, f)
		if err != nil {
			panic(fmt.Errorf("Failed to parse the default with type '%s': %v", t, err))
		}
		return val, !noDefault
	}

}

//func (s *ValueProvider) Parse(defaultExpr string) interface{} {
//	items := strings.SplitN(defaultExpr, ":", 2)
//	provider := s.parsers["str"]
//	if len(items) == 2 {
//		provider, items = s.parsers[items[1]], items[:1]
//	}
//	val := provider(items[0])
//	return val
//}

func (i *dummyFileInfo) Name() string {
	return i.name
}

func (i *dummyFileInfo) IsDir() bool {
	return i.isDir
}

func (i *dummyFileInfo) Size() int64 {
	return 0
}

func (i *dummyFileInfo) Mode() os.FileMode {
	if i.isDir {
		return os.ModeDir
	}
	return 0
}

func (i *dummyFileInfo) ModTime() time.Time {
	return time.Time{}
}

func (i *dummyFileInfo) Sys() interface{} {
	return i.sys
}

// NewJsonWalker returns an iterator over contents of the json source.
//
// Iterator provided by NewJsonWalker supports the same semantics as standard
// `filepath.Walk`.
func NewJsonWalker(jsonSource string) (*JsonWalker, error) {
	walker := new(JsonWalker)
	var root interface{}
	if err := json.Unmarshal([]byte(jsonSource), &(root)); err != nil {
		return nil, err
	}
	walker.fs = root
	return walker, nil
}

// NewObjWalker returns an iterator over contents of a composite object.
//
// Iterator provided by NewObjWalker supports the same semantics as standard
// `filepath.Walk`.
//
// The only nodes that can be inspected in depth are generic map and generic
// array, namely `map[string]interface{}` and `[]interface{}`.
func NewObjWalker(root interface{}) *JsonWalker {
	walker := new(JsonWalker)
	walker.fs = root
	return walker
}

// Walk implements similar behavior to `filepath.Walk`.
func (w *JsonWalker) Walk(path string, walkFunc filepath.WalkFunc) error {
	node, err := seek(w.fs, path)
	if err != nil {
		return err
	}
	walk(node, path, walkFunc)
	return nil
}

// Seek walks through walker's target object until specific path is reached,
// returning handle to data at that location.
//
// Seek tries its best to find value at given path. Failure to reach the path
// is indicated with error value of `NotFound`.
func (w *JsonWalker) Seek(seekPath string) (interface{}, error) {
	return seek(w.fs, seekPath)
}

func seek(root interface{}, seekPath string) (interface{}, error) {
	var result interface{}
	resultSet := false
	walk(root, "/", func(path string, info os.FileInfo, _ error) error {
		if result != nil {
			return filepath.SkipDir
		} else if path == seekPath {
			result = info.Sys()
			resultSet = true
			return filepath.SkipDir
		}
		return nil
	})
	if resultSet {
		return result, nil
	}
	return nil, NotFound
}

func basename(path string) string {
	base := filepath.Base(path)
	if base == "." {
		return "/"
	}
	return base
}

func walk(node interface{}, path string, walkFunc filepath.WalkFunc) error {
	var err error
	if dirNode, isDir := node.(map[string]interface{}); isDir {
		err = walkFunc(path, &dummyFileInfo{basename(path), true, dirNode}, nil)
		if err == filepath.SkipDir {
			return nil
		}
		for k, subNode := range dirNode {
			err = walk(subNode, filepath.Join(path, k), walkFunc)
			if err == filepath.SkipDir {
				return nil
			}
		}
	} else if dirNode, isDir := node.([]interface{}); isDir {
		err = walkFunc(path, &dummyFileInfo{basename(path), true, dirNode}, nil)
		if err == filepath.SkipDir {
			return nil
		}
		for k, subNode := range dirNode {
			err = walk(subNode, filepath.Join(path, strconv.Itoa(k)), walkFunc)
			if err == filepath.SkipDir {
				return nil
			}
		}
	} else {
		err = walkFunc(path, &dummyFileInfo{basename(path), false, node}, nil)
		if err == filepath.SkipDir {
			return err
		}
	}
	return nil
}
