#!/bin/bash
resty -I lib/ bin/gen_config.lua > config/nginx.development.conf

openresty -p `pwd`/ -c `pwd`/config/nginx.development.conf | awk '
  /\[400\]/{print "\033[33m" $0 "\033[39m"; next}
  /\[500\]/{print "\033[33m" $0 "\033[39m"; next}
  /\[404\]/{print "\033[33m" $0 "\033[39m"; next}
  /\[200\]/{print "\033[32m" $0 "\033[39m"; next}
  1 {print "\033[31m" $0 "\033[39m"; next}
'