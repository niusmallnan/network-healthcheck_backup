network-healthcheck
========

A microservice that does micro things.


## Architecture

![](https://ws3.sinaimg.cn/mw1024/006tNc79ly1fhznl7kwvlj31ik0widij.jpg)

## How to check
There are three detection methods, allfor containers with the `managed` network option:  
1. Ping, send 5 Ping packets to the target container to see if there is a packet loss.
2. Arping, send 5 arp requests to the target container to see if the returned mac address is consistent.
3. HTTP, request http ping for the router container to see if the networking service is ok.

In general, all containers are ping and arping detection, router containers will be additional http detection.

The network-healthcheck will turn red in Rancher UI when any container's network is unreachable.
Then you can look at the network-healthcheck log and it will tell you which container it is.

## How to deploy
Here is a catalog repo [network-healthcheck-catalog](https://github.com/niusmallnan/network-healthcheck-catalog).

You can enable it in Rancher. If you use ipsec/vxlan, you need to enable router-http-check.

## Building

`make`


## Running

`./bin/network-healthcheck`

## License
Copyright (c) 2014-2017 [Rancher Labs, Inc.](http://rancher.com)

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

[http://www.apache.org/licenses/LICENSE-2.0](http://www.apache.org/licenses/LICENSE-2.0)

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
