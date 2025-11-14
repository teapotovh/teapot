package net

// For jool, we basically just have to do:

// jool_siit instance remove teapotsiit0
// modprobe jool
// jool instance add "teapot" --netfilter --pool6 64:ff9b::/96
// jool -i teapot pool4 add --tcp <node local ipv4> 1024-65535
// jool -i teapot pool4 add --udp <node local ipv4> 1024-65535
// jool -i teapot pool4 add --icmp <node local ipv4> 1024-65535
