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
	"id":"!!",
	"name":"!!",
	"aliases":[
	],
	"labels":{
		"io.kubernetes.pod.name":"__tmpl|/labels/io_kubernetes_pod_name/value||str|no_default=",
		"io.kubernetes.container.name":"__tmpl|/labels/io_kubernetes_container_name/value||str|no_default=",
		"io.kubernetes.pod.uid":"__tmpl|/labels/io_kubernetes_pod_uid/value||str|no_default=",
		"io.kubernetes.pod.namespace":"__tmpl|/labels/io_kubernetes_pod_namespace/value||str|no_default="
	},
	"subcontainers":[
	],
	"spec":{
		"creation_time":"__tmpl|/creation_time|2016-05-16T03:25:47Z|str",
		"labels":{
			"io.kubernetes.pod.name":"__tmpl|/labels/io_kubernetes_pod_name/value||str|no_default=",
			"io.kubernetes.container.name":"__tmpl|/labels/io_kubernetes_container_name/value||str|no_default=",
			"io.kubernetes.pod.uid":"__tmpl|/labels/io_kubernetes_pod_uid/value||str|no_default=",
			"io.kubernetes.pod.namespace":"__tmpl|/labels/io_kubernetes_pod_namespace/value||str|no_default="
		},
		"has_cpu":true,
		"cpu":{
			"limit":2,
			"max_limit":2,
			"mask":"0-1"
		},
		"has_memory":true,
		"memory":{
			"limit":"__tmpl|/cgroups/memory_stats/stats/limit_in_bytes|0|int",
			"swap_limit":"__tmpl|/cgroups/memory_stats/stats/swap_limit_in_bytes|0|int"
		},
		"has_network":true,
		"has_filesystem":true,
		"has_diskio":false,
		"has_custom_metrics":false,
		"image":"__tmpl|/image_name|docker|str"
	},
	"stats":[
		{
			"timestamp":"!!",
			"cpu":{
				"usage":{
					"total":"__tmpl|/cgroups/cpu_stats/cpu_usage/total_usage|0|int",
					"user":"__tmpl|/cgroups/cpu_stats/cpu_usage/usage_in_usermode|0|int",
					"system":"__tmpl|/cgroups/cpu_stats/cpu_usage/usage_in_kernelmode|0|int"
				},
				"load_average":"__tmpl|/sched_load|0|int"
			},
			"diskio":{
			},
			"memory":{
				"usage":"__tmpl|/cgroups/memory_stats/usage/usage|0|int",
				"cache":"__tmpl|/cgroups/memory_stats/cache|0|int",
				"rss":"__tmpl|/cgroups/memory_stats/stats/rss|0|int",
				"working_set":"__tmpl|/cgroups/memory_stats/stats/working_set|0|int",
				"failcnt":0,
				"container_data":{
					"pgfault":"__tmpl|/cgroups/memory_stats/stats/pgfault|0|int",
					"pgmajfault":"__tmpl|/cgroups/memory_stats/stats/pgmajfault|0|int"
				},
				"hierarchical_data":{
					"pgfault":0,
					"pgmajfault":0
				}
			},
			"network":{
				"name":"__tmpl|/network/name||str",
				"rx_bytes":"__tmpl|/network/rx_bytes|0|int",
				"rx_packets":"__tmpl|/network/rx_packets|0|int",
				"rx_errors":"__tmpl|/network/rx_errors|0|int",
				"rx_dropped":"__tmpl|/network/rx_dropped|0|int",
				"tx_bytes":"__tmpl|/network/tx_bytes|0|int",
				"tx_packets":"__tmpl|/network/tx_packets|0|int",
				"tx_errors":"__tmpl|/network/tx_errors|0|int",
				"tx_dropped":"__tmpl|/network/tx_dropped|0|int",
				"interfaces":[
					{
						"name":"__tmpl|/name||str",
						"rx_bytes":"__tmpl|/rx_bytes|0|int",
						"rx_packets":"__tmpl|/rx_packets|0|int",
						"rx_errors":"__tmpl|/rx_errors|0|int",
						"rx_dropped":"__tmpl|/rx_dropped|0|int",
						"tx_bytes":"__tmpl|/tx_bytes|0|int",
						"tx_packets":"__tmpl|/tx_packets|0|int",
						"tx_errors":"__tmpl|/tx_errors|0|int",
						"tx_dropped":"__tmpl|/tx_dropped|0|int"
					}
				],
				"tcp":{
					"Established":"__tmpl|/connection/tcp/established|0|int",
					"SynSent":"__tmpl|/connection/tcp/syn_sent|0|int",
					"SynRecv":"__tmpl|/connection/tcp/syn_recv|0|int",
					"FinWait1":"__tmpl|/connection/tcp/fin_wait1|0|int",
					"FinWait2":"__tmpl|/connection/tcp/fin_wait2|0|int",
					"TimeWait":"__tmpl|/connection/tcp/time_wait|0|int",
					"Close":"__tmpl|/connection/tcp/close|0|int",
					"CloseWait":"__tmpl|/connection/tcp/close_wait|0|int",
					"LastAck":"__tmpl|/connection/tcp/last_ack|0|int",
					"Listen":"__tmpl|/connection/tcp/listen|0|int",
					"Closing":"__tmpl|/connection/tcp/closing|0|int"
				},
				"tcp6":{
					"Established":"__tmpl|/connection/tcp/established|0|int",
					"SynSent":"__tmpl|/connection/tcp/syn_sent|0|int",
					"SynRecv":"__tmpl|/connection/tcp/syn_recv|0|int",
					"FinWait1":"__tmpl|/connection/tcp/fin_wait1|0|int",
					"FinWait2":"__tmpl|/connection/tcp/fin_wait2|0|int",
					"TimeWait":"__tmpl|/connection/tcp/time_wait|0|int",
					"Close":"__tmpl|/connection/tcp/close|0|int",
					"CloseWait":"__tmpl|/connection/tcp/close_wait|0|int",
					"LastAck":"__tmpl|/connection/tcp/last_ack|0|int",
					"Listen":"__tmpl|/connection/tcp/listen|0|int",
					"Closing":"__tmpl|/connection/tcp/closing|0|int"
				}
			},
			"filesystem":[
				{
					"device":"/dev/sdx1",
					"type":"vfs",
					"capacity":0,
					"usage":0,
					"base_usage":0,
					"available":0,
					"inodes_free":0,
					"reads_completed":0,
					"reads_merged":0,
					"sectors_read":0,
					"read_time":0,
					"writes_completed":0,
					"writes_merged":0,
					"sectors_written":0,
					"write_time":0,
					"io_in_progress":0,
					"io_time":0,
					"weighted_io_time":0
				}
			]
		}
	]
}
`
