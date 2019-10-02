/*
 * skogul, using config file
 *
 * Copyright (c) 2019 Telenor Norge AS
 * Author(s):
 *  - Kristian Lyngstøl <kly@kly.no>
 *
 * This library is free software; you can redistribute it and/or
 * modify it under the terms of the GNU Lesser General Public
 * License as published by the Free Software Foundation; either
 * version 2.1 of the License, or (at your option) any later version.
 *
 * This library is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the GNU
 * Lesser General Public License for more details.
 *
 * You should have received a copy of the GNU Lesser General Public
 * License along with this library; if not, write to the Free Software
 * Foundation, Inc., 51 Franklin Street, Fifth Floor, Boston, MA
 * 02110-1301  USA
 */

/*
cmd/skogul parses a json-based config file and starts skogul.
*/
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/KristianLyng/skogul/config"
	"github.com/KristianLyng/skogul/receiver"
	"github.com/KristianLyng/skogul/sender"
	"github.com/KristianLyng/skogul/transformer"
)

var ffile = flag.String("f", "~/.config/skogul.json", "Path to skogul config to read.")
var fhelp = flag.Bool("help", false, "Print more help")
var fconf = flag.Bool("show", false, "Print the parsed JSON config instead of starting")
var fman = flag.Bool("make-man", false, "Output RST documentation suited for rst2man")

// man generates an RST document suited for converting to a manual page
// using rst2man. The RST document itself is also valid, but some short
// cuts have been made, e.g., cutting long lines is not done, so the
// raw rst document might seem a bit rough, but translated to a manual page
// it looks fine.
//
// Also includes help for all senders and receivers, and uses flag to print
// the command line flag options as well.
func man() {
	fmt.Print(`
======
skogul
======

------
Skogul
------

:Manual section: 1
:Authors: Kristian Lyngstøl <kly@kly.no>

SYNOPSIS
========

::

	skogul -f config-file [-show]
	
	skogul [-help | -show | -make-man]

DESCRIPTION
===========

Skogul is a generic tool for moving metric data around. It can serve as a
collector of data, but is primarily designed to be a framework for building
bridges between data collectors and storage engines.

These bridges can be simple - accept data on HTTP, write to influxdb - or
complex: Accept data on unencrypted http, batch data together, forward it
to a remote skogul-instance over a password-protected, encrypted HTTPS
channel, if that fails, write to a local queue and retry periodically.

To facilitate this, Skogul has three core components:

1. Receivers acquire raw data
2. Handlers turns raw data into meaningful content
3. Senders determine what happens to the data

A single instance of Skogul must have at least one receiver, but can have
multiple. It also, typically, must have at least one handler and sender.

Senders come in two distinct but interchangeable variants: Storage-oriented
senders are used to send the data to some external resource, e.g., a time
series database like InfluxDB. Utility-oriented senders are used to add
logic, such as error handling or duplicating data to multiple storage
systems.

There are more examples in the the "examples/" directory.

OPTIONS
=======

`)

	f := flag.CommandLine
	f.VisitAll(func(fl *flag.Flag) {
		s := fmt.Sprintf("``-%s``", fl.Name)
		name, usage := flag.UnquoteUsage(fl)
		if len(name) > 0 {
			s += " " + name
		}
		s += "\n\t"
		s += strings.ReplaceAll(usage, "\n", "\n\t")
		if fl.DefValue != "" {
			s += fmt.Sprintf(" (default %v)", fl.DefValue)
		}
		fmt.Print(s, "\n\n")
	})

	fmt.Print(`
CONFIGURATION
=============

Configuration of skogul is done with a json config file, referenced with
the -f option. You need to specify at least one receiver, handler and
sender to make something sensible.

The base configuration set is::

  {
    "receivers": {
      "xxx": {
        "type": "type-of-receiver",
        type-specific-options
      },
      "other-receiver...": ...
    },
    "handlers": {
      "yyy": {
        "parser": "json", // other options might come
        "transformers": [...],
        "sender": "reference-to-sender"
      }
    },
    "transformers": {
      "rrr": {
        "type": "type-of-transformer",
        type-specific-options
      }
    },
    "senders": {
      "zzz": {
        "type": "type-of-sender",
        type-specific-options
      },
      "qqq": {
        "type": "type-of-sender",
        type-specific-options
      },
      ...
    }
  }

In the above pseudo-config, "xxx", "yyy", "zzz", "rrr", and "qqq" are
arbitrary names you chose that are how to reference that specific item
within the same configuration. The "type" field references what
implementation to use - each implementation have different configuration
options. You can specify as many senders, receivers and handlers as you
want, and they can cross-reference each other.

Upon start-up, all receivers are started.

It is valid to have multiple receivers use the same handler. It is also
valid for multiple senders to reference the same sender. It is up to the
operator to avoid setting up feedback loops.

The only parser available today is the JSON parser. Only two transformers
exists, and to simplify configuration, the "templater" transformer does
not have to be explicitly defined to be referenced.

The documentation for each sender and receiver also lists all options. In
general, you do not need to specify all options.

CONFIGURATION DATA TYPES
========================

Data types are parsed into Go types. In most cases, the the type is self
explanatory (e.g: a "string" is a typical text string, "int" is an integer,
and so on).

However, here are some examples that might not be obvious.

HandlerRef
	This is a text string referencing a named handler, specified in
	"handlers".

SenderRef
	A text string referencing a named sender, specified in "senders".

[]string
	An array of text strings. E.g. ["foo","bar"].

[]*skogul.HandlerRef
	An array of SenderRef, so similar to the above ["foo", "bar"], where "foo"
	and "bar" are senders named in the "senders" section of the configuration.

map[string]*skogul.HandlerRef
	This is a map of strings to handler references. For example, { "/some/path": "aHandler",
	"/other/path": "bHandler"}.

interface{}
	This is a generic "anything"-structure that can hold any arbitrary
	value. Can be any structure or variable, including nested
	variables. Used in the data/metadata transformers, among others.

SENDERS
=======

The following senders exist.

`)
	senders := []string{}
	for idx := range sender.Auto {
		if sender.Auto[idx].Name != idx {
			continue // alias
		}
		senders = append(senders, idx)
	}
	sort.Strings(senders)
	for _, s := range senders {
		sh, _ := config.HelpSender(s)
		thingMan(sh)
	}
	fmt.Print(`
RECEIVERS
=========

The following receivers exist.

`)
	receivers := []string{}
	for idx := range receiver.Auto {
		if receiver.Auto[idx].Name != idx {
			continue // alias
		}
		receivers = append(receivers, idx)
	}
	sort.Strings(receivers)
	for _, r := range receivers {
		sh, _ := config.HelpReceiver(r)
		thingMan(sh)
	}
	fmt.Print(`

TRANSFORMERS
============

Transformers are the only tools that can actively modify a metric. See the
"HANDLERS" section for more discussion. Note that the "templater" transformer
does not need to be defined - if a handler lists "templater", one will be
created behind the scenes. The available transformers are:

`)
	transformers := []string{}
	for idx := range transformer.Auto {
		if transformer.Auto[idx].Name != idx {
			continue // alias
		}
		transformers = append(transformers, idx)
	}
	sort.Strings(transformers)
	for _, r := range transformers {
		sh, _ := config.HelpTransformer(r)
		thingMan(sh)
	}
	fmt.Print(`
HANDLERS
========

There is only one type of handler. It accepts three arguments: A parser to
parse data, a list of optional transformers, and the first sender that will
receive the parsed container(s).

Currently the only valid parser is "json" and only two transformers exist.
The "templating" transformer does not need to be explicitly defined to be
referenced, since it has no settings.

JSON parsing
------------

If the "json" parser is used (Currently the only one available), data sent
to Skogul will be parsed to fit the internal data model of Skogul. The JSON
representation is roughly thus::

  {
    "template": { 
      "timestamp": "iso8601-time",
      "metadata": { 
        "key": value, 
        ...
      },
      "data": {
        "key": value,
        ...
      }
    },
    "metrics": [
      {
        "timestamp": "iso8601-time",
        "metadata": { 
          "key": value, 
          ...
        },
        "data": {
          "key": value,
          ...
        }
      },
      { ...}
    ]
  }

The "template" is optional, see the "Templater"-documentation above for an
in-depth description.

The primary difference between metadata and data is searchability,
and it will depend on storage engines. Typically this means the name
of a server is metadata, but the load average is data. Skogul itself
does not much care.

Templating
----------

The templating-transformer is useful for adding identical fields to all
metrics in a collection. If a template is provided, and the
templater-transformer is applied, all metrics are initialized with whatever
value the template came with.

This is inteded for when you are sending multiple metrics that share
certain attributes, e.g, they are all from the same machine and all
collected at the same time. Or they are all from the same data center
or region.

Templates are shallow. If your metric has nested fields, they will not
be merged with what the template provides. For example::

   {
     "template": {
       "timestamp": "2019-09-27T15:42:00Z",
       "metadata": {
         "site": "naboo",
         "machine": {
           "os": "Debian"
         }
       }
     },
     "metrics": [
       {
         "metadata": {
           "machine": {
             "hostname": "r2d2"
           }
         },
         "data": {
           "something": "blah"
         }
       },
       {
         "metadata": {
           "machine": {
             "hostname": "c3po"
           }
         },
         "data": {
           "something": "duck"
         }
       }
     ]
   }

Here, the template provides three items: a timestamp, the "site" field and
the "machine" field of metadata. Once transformed, the result will be::

   {
     "metrics": [
       {
         "timestamp": "2019-09-27T15:42:00Z",
         "metadata": {
           "site": "naboo",
           "machine": {
             "hostname": "r2d2"
           }
         },
         "data": {
           "something": "blah"
         }
       },
       {
         "timestamp": "2019-09-27T15:42:00Z",
         "metadata": {
           "site": "naboo",
           "machine": {
             "hostname": "c3po"
           }
         },
         "data": {
           "something": "duck"
         }
       }
     ]
   }

Since each metric also provided a "machine"-field, it overwrote the value
from the template, even if there were no overlapping fields.


EXAMPLES
========

A minimalistic example that accepts data on HTTP and prints it to standard
output::

  { 
    "receivers": { 
      "api": { 
        "type": "http", 
        "address": ":8080", 
        "handlers": { "/": "myhandler" }
      }
    },
    "handlers": {
      "myhandler": {
        "parser": "json", 
        "transformers": ["templater"], 
        "sender": "mysender"
      }
    },
    "senders": {
      "mysender": {
        "type": "debug"
      }
    }
  }

The following specifies an insecure HTTP-based receiver that will wait up
to 5 seconds or 1000 metrics before writing data to InfluxDB::

  {
    "receivers": {
      "api": {
        "type": "http",
        "address": "[::1]:8080",
        "handlers": {
          "/": "jsontemplating"
        }
      }
    },
    "handlers": {
      "jsontemplating": {
        "parser": "json",
        "transformers": [ "templater" ],
        "sender": "batch"
      }
    },
    "senders": {
      "batch": {
        "type": "batch",
        "interval": "5s",
        "threshold": 1000,
        "next": "influx"
      },
      "influx": {
        "type": "influx",
        "URL": "http://[::1]:8086/write?db=testdb",
        "measurement": "demo",
        "Timeout": "10s"
      }
    }
  }

To add a metadata field to signal where data came from before passing it on
to a central instance::

  {
    "receivers": {
      "local": {
        "type": "http",
        "address": "[::1]:8080",
        "handlers": {
          "/": "jsontemplating"
        }
      }
    },
    "transformers": {
      "origin": {
        "type": "metadata",
        "set": {
          "dc": "bergen1",
          "collector": "serverX"
        }
      }
    },
    "handlers": {
      "jsontemplating": {
        "parser": "json",
        "transformers": [ "templater","metadata" ],
        "sender": "batch"
      }
    },
    "senders": {
      "batch": {
        "type": "batch",
        "interval": "5s",
        "threshold": 1000,
        "next": "central"
      },
      "central": {
        "type": "http",
        "url": "https://bergen1X:hunter2@central-skogul.example.com/",
        "Timeout": "10s"
      }
    }
  }

More examples are provided in the examples/ directory of the Skogul source
package.

SEE ALSO
========

https://github.com/KristianLyng/skogul

BUGS
====

Configuration parsing doesn't provide very helpful errors, and silently
ignores keys/variables that are not used in a specific context.

Workaround: Use the "-show" option to display the parsed configuration.

COPYRIGHT
=========

This document is licensed under the same license as Skogul itself, which
happens to be GPLv2 (or later). See LICENSE for details.

* Copyright (c) 2019 - Telenor Norge AS

`)

}

// thingMan is thus named because of reasons. It prints RST-formatted
// documentation for a sender or receiver, whatever config.Help has.
func thingMan(thing config.Help) {
	fmt.Printf("%s\n", thing.Name)
	for l := len(thing.Name); l > 0; l-- {
		fmt.Print("-")
	}
	fmt.Printf("\n\n")
	fmt.Printf("%s\n\n", thing.Doc)
	if thing.Aliases != "" {
		fmt.Printf("Aliases: %s\n\n", thing.Aliases)
	}
	fields := []string{}
	doit := false
	for n := range thing.Fields {
		fields = append(fields, n)
		doit = true

	}
	if doit {
		fmt.Printf("Settings:\n\n")
	}
	sort.Strings(fields)
	for _, n := range fields {
		f := thing.Fields[n]
		fmt.Printf("``%s - %s``\n\t", strings.ToLower(n), f.Type)
		fmt.Printf("%s\n\n", f.Doc)
		if f.Example != "" {
			fmt.Printf("\tExample(s): %s\n\n", f.Example)
		}
	}
}

// Console width :D
const helpWidth = 66

// prettyPrint is a relic that wraps lines in a table.
func prettyPrint(scheme string, desc string) {
	fmt.Printf("%11s |", scheme)
	fields := strings.Fields(desc)
	l := 0
	for _, w := range fields {
		if (l + len(w)) > helpWidth {
			l = 0
			fmt.Printf("\n%11s |", "")
		}
		fmt.Printf(" %s", w)
		l += len(w) + 1
	}
	fmt.Printf("\n")
}

// help prints the regular command line usage, and lists all receivers and
// senders.
func help() {
	flag.Usage()
	fmt.Println("\nSenders:")
	for idx, sen := range sender.Auto {
		prettyPrint(idx, sen.Help)
	}
	fmt.Println("\nReceivers:")
	for idx, rcv := range receiver.Auto {
		prettyPrint(idx, rcv.Help)
	}
	fmt.Println("\nYou can also see the skogul manual page. It can be generated with `./skogul -make-man > foo; rst2man < foo > skogul.1; man ./skogul.1'.")
}

func main() {
	flag.Parse()
	if *fhelp {
		help()
		os.Exit(0)
	}
	if *fman {
		man()
		os.Exit(0)
	}

	c, err := config.File(*ffile)
	if err != nil {
		log.Fatal(err)
	}

	if *fconf {
		out, err := json.MarshalIndent(c, "", "  ")
		if err != nil {
			panic(err)
		}
		fmt.Println(string(out))
		os.Exit(0)
	}

	// I know there's a simpler way of saying "start these things and
	// wait", but I really can't be bothered to look it up right this
	// moment.
	foo := make([]chan int, len(c.Receivers))
	i := 0
	for _, r := range c.Receivers {
		i++
		go func(r *config.Receiver, i int) {
			r.Receiver.Start()
			foo[i] <- 1
		}(r, i)
	}
	for ii := 0; ii < len(foo); ii++ {
		<-foo[ii]
	}
}
