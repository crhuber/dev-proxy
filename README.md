# Dev-Proxy

Simple port forwarding for localhost development. 


## What is it

Dev-Proxy makes developing on localhost easy by assiging a dns hostname and virtual ip for your localhost project. 
No more trying to remember what localhost port your local dev environment is running on: connect to your easy to remember hostname on port 80 
Skip complicated nginx port-forwarding or dnsmasq configs.


## How it works

Dev-proxy does the following things

1. Sets up a virtual ip (127.0.0.2) and creates an alias for it on the lo0 loopback adapter
2. Creates a port forwarding rule to forward traffic destined for the virutal ip (127.0.0.2) on port 80 to be forwarded to a localhost port of your choosing (8080 by default)
3. Edits /etc/hosts and adds a friendly hostname of your choosing pointing to the virtual ip

This is roughly analogous to the way a Kubernetes Service works where kube-proxy sets up a virtual ip, sets up ip tables rules and assigns a hostname in core-dns for the virtual ip.


## How to use 

### Add

Say you have an application running on port 8080 localhost and you'd like to instead connect to it by hostname dev.internal on port 80.
First add it to your confg file by running the add command

Run:

`> sudo dev-proxy add -host dev.internal -port 8080`

```
==> Dev proxy: Config file updated!
```

Flags:

```
  -host string
        hostname that will resolve to a virtual ip (default "dev.internal")
  -port int
        local port to proxy to (default 8080)
```

You can add up to 254 new entries.

### Up 

Now that you have added the config for application. Run the dev-proxy

Run:

`> sudo dev-proxy up`

```
Activating dev-proxy...

[dev.internal]
==> Setting up virtual ip: 127.0.0.2
==> Updating hostfile: dev.internal
Hostfile entry active
dev.internal => 127.0.0.2:80 => 127.0.0.1:8080 

==> Setting up port forwarding
Port forwarding: configured

Dev proxy: running!
```

### Status

To check the status of the virtual ip, port-forwarding and dns config run

`> sudo dev-proxy status`

```
==> Hosts file:
127.0.0.1       localhost
127.0.0.2       dev.internal

==> Loopback interface lo0 addresses:
127.0.0.1
127.0.0.2

==> Port forwarding rules:
rdr pass inet proto tcp from any to 127.0.0.2 port = 80 -> 127.0.0.1 port 8080
```

### Reset

Once you are done you can remove the loopback aliases by running

`> sudo dev-proxy reset`

```
==> Loopback interface lo0 addresses:
Removing alias: 127.0.0.2
```


## FAQ

- Q: Where is the config file located
   - A: `~/.devproxy/config.toml`

- Q: Will the port-forwading rules persist across reboots?
    - A: No, all changes will be reset after a reboot

- Q: What is it actually doing under the hood?
    - A: It basically just runs these commands:

    ```
    sudo ifconfig lo0 alias 127.0.0.2
    echo "rdr pass inet proto tcp from any to 127.0.0.2 port 80 -> 127.0.0.1 port 8080" | sudo pfctl -ef -
    echo "127.0.0.1 dev.internal" >> /etc/hosts
    ```

- Q: Couldn't this just have been a bash script?
    - A: Probably



## Contributing

If you find bugs, please open an issue first. 

If you have feature requests, I probably will not honor it because this project is being built mostly to suit my personal workflow and preferences.
