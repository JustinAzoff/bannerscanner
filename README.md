A simple tcp port scanner and banner grabber.  Logs output as json.

You probably want to be using [massscan](https://github.com/robertdavidgraham/masscan).

Usage:

          --banner-timeout duration   timeout when fetching banner (default 2s)
          --debug                     sets log level to debug
      -x, --exclude stringSlice       cidr blocks to exclude
          --parallel                  Scan multiple ports on each host in parallel
      -p, --port stringSlice          ports to scan. ex: 80,443,8000-8100
          --pretty                    use pretty logs
          --rate int                  rate in attempts/sec (default 1000)
          --timeout duration          Scan connection timeout (default 2s)

Example:

    $ ./bannerscanner -p 22,53 192.168.2.0/28
    {"level":"info","state":"open","host":"192.168.2.10","port":22,"banner":"SSH-2.0-nope\n","time":"2018-05-16T16:46:17-04:00","message":"found service"}
    {"level":"info","state":"open","host":"192.168.2.11","port":22,"banner":"SSH-2.0-nope\r\nProtocol mismatch.\n","time":"2018-05-16T16:46:17-04:00","message":"found service"}
    {"level":"info","state":"open","host":"192.168.2.1","port":22,"banner":"SSH-2.0-nope\r\n","time":"2018-05-16T16:46:17-04:00","message":"found service"}
    {"level":"info","state":"open","host":"192.168.2.1","port":53,"banner":"","time":"2018-05-16T16:46:19-04:00","message":"found service"}
