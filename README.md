# cdpproxy

cdpproxy is a websocket proxy to logs all messages on cdp connections.

## Install and build

The program requires [Go](https://go.dev] installed.

```console
$ go build
```

## Usage

You can use the proxy with an existing browser like [Lightpanda](https://lightpanda.io).

By default cdpproxy listens websocket incoming connections on `127.0.0.1:9222`.
You can use the `--addr` option or `CDP_ADDRESS` env var to change the configuration.

By default cdpproxy connects to the browser on `ws://127.0.0.1:9223`.
You can give another url by passing it as the first argument.

### Usage with cdp client

You must configure your client to connect to the cdpproxy address.

Example with Puppeteer
```js
'use scrict'

import puppeteer from 'puppeteer-core';

const browser = await puppeteer.connect({
  browserWSEndpoint: "ws://127.0.0.1:9222",
});
```

Example with Playwright
```js
import { chromium } from 'playwright';

const browser = await chromium.connectOverCDP({
    endpointURL: 'ws://127.0.0.1:9222',
});
```
### Usage with Lightpanda

Start Lightpanda browser.
```
$ ./ligthpanda serve --port 9223
```

Start cdpproxy and run your cdp script to display messages.
```
$ ./cdpproxy
> {"method":"Browser.getVersion","id":1}
< {"id":1,"result":{"protocolVersion":"1.3","product":"Chrome/124.0.6367.29","revision":"@9e6ded5ac1ff5e38d930ae52bd9aec09bd1a68e4","userAgent":"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7
) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36","jsVersion":"12.4.254.8"}}
> {"method":"Target.getBrowserContexts","id":2}
< {"id":2,"result":{"browserContextIds":[]}}
```

### Usage with Chromium headless

Start Chromium browser.
```
$ chromium --headless=new --remote-debugging-port=9223

DevTools listening on ws://127.0.0.1:9223/devtools/browser/696603b7-6ac7-4f0a-99ad-cd356c7f8b7d
```

Start cdpproxy with the ws url and run your cdp script to display messages.
```
$ ./cdpproxy ws://127.0.0.1:9223/devtools/browser/696603b7-6ac7-4f0a-99ad-cd356c7f8b7d
> {"method":"Browser.getVersion","id":1}
< {"id":1,"result":{"protocolVersion":"1.3","product":"Chrome/124.0.6367.29","revision":"@9e6ded5ac1ff5e38d930ae52bd9aec09bd1a68e4","userAgent":"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7
) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36","jsVersion":"12.4.254.8"}}
> {"method":"Target.getBrowserContexts","id":2}
< {"id":2,"result":{"browserContextIds":[]}}
> {"method":"Target.setDiscoverTargets","params":{"discover":true,"filter":[{}]},"id":3}
< {"id":3,"result":{}}
```
