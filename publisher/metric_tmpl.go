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

const builtinMetricTemplate = `{
"id": "!!",
"name": "!!",
"aliases": [
],
"namespace": "docker",
"labels": {
},
  "subcontainers": [],
"spec": {
 "creation_time": "/creation_time",
 "labels": {
 },
 "has_cpu": true,
 "cpu": {
 },
 "has_memory": true,
 "memory": {
 },
 "has_network": true,
 "has_filesystem": false,
 "has_diskio": true,
 "has_custom_metrics": true,
 "custom_metrics":[],
 "image": "/image_name"
},
"stats": [
 {
  "timestamp": "!!",
  "custom_metrics": {
   "SNAP": [
   ]
  },
  "cpu": {
   "usage": {
    "total": "/stats/cpu_stats/cpu_usage/total_usage"
   }
  },
  "memory": {
   "usage": "/stats/memory_stats/usage/usage",
   "container_data": {
    "pgfault": "/stats/memory_stats/stats/pgfault",
    "pgmajfault": "/stats/memory_stats/stats/pgmajfault"
   }
  },
  "network": {
   "rx_bytes": "/stats/network/rx_bytes",
   "rx_errors": "/stats/network/rx_errors",
   "tx_bytes": "/stats/network/tx_bytes",
   "tx_errors": "/stats/network/tx_errors"
  }
 }
]
}
`
