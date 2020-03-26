# Redirect to proxy

`redirect-cli.exe` redirects transparently http and https traffic on your local windows system to a http proxy.

It's uses the Windows Filtering Platform (WFP) to modify IP packets inside the TCP/IP stack. 

Access to WFP is implemented by [WinDivert](https://github.com/basil00/Divert) and it's Go wrapper [GoDivert](https://github.com/williamfhe/godivert)

## Installation
- Download binary from releases
- Download [WinDivert-Package Version 1.4.x](https://github.com/basil00/Divert/releases/download/v1.4.3/WinDivert-1.4.3-A.zip)
- Exctract WinDivert zip file 
- Copy `WinDivert64.sys` and `WinDivert.dll` from `x86_64` folder next to `redirect-cli.exe`

## Running 

Run from terminal with elevated privileges (administrative permissions)
```bash
redirect -proxy <Proxy-Host>:<Port>
```