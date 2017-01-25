#Status

This repository is no longer being actively maintained by Intel. Work has instead shifted to the [snap-plugin-publisher-heapster](https://github.com/intelsdi-x/snap-plugin-publisher-heapster) repository.

## snap publisher with embedded REST API server

### Example configuration

Here is a sample task manifest utilizing the heapster publisher plugin:

```
  version: 1
  schedule:
    type: "simple"
    interval: "10s"
  workflow:
    collect:
      metrics:
        /intel/linux/docker/*/cpu_stats/cpu_usage/total_usage: {}
        /intel/linux/docker/*/memory_stats/usage/usage: {}
        /intel/linux/docker/*/memory_stats/stats/pgfault: {}
        /intel/linux/docker/*/memory_stats/stats/pgmajfault: {}
      process:
        -
          plugin_name: "passthru"
          process: null
          publish:
            -
              plugin_name: "heapster"
              config:
                stats_depth: 0
                server_addr: 127.0.0.1
                server_port: 8777
                stats_span: "10m"
                export_tmpl_file: "builtin"
```

Explanation:
* this will setup heapster publisher to expose REST server at address 127.0.0.1:8777
* publisher will keep a list of stats spanning 10 minutes
* no limit on number of stats (`stats_depth: 0` means _no limit_)
* will use builtin template file for metrics; (might have given a path
to specific template json)

### Known issues

Heapster publisher REST server is unable to restart when the plugin is
reloaded. REST server is started at first call to `PublishMetrics()`.

The workaround is to submit a bootstrap task that will keep the 
publisher loaded, e.g.:
```
  version: 1
  schedule:
    type: "simple"
    interval: "1h"
  workflow:
    collect:
      metrics:
        /intel/linux/mock/foo: {}
      process:
        -
          plugin_name: "passthru"
          process: null
          publish:
            -
              plugin_name: "heapster"
              config:
                stats_depth: 0
                server_port: 8777
                stats_span: "10m"
                export_tmpl_file: "builtin"
                tstamp_delta: "1h"
```
but note that the config given in bootstrap's manifest will be kept for
all further invocations of publisher.
