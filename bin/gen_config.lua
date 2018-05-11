local liluat = require "liluat"

local file = io.open("config/nginx.conf", 'r')
local tmpl = liluat.compile(file:read("*a"))

local manifest = require "kubelang"

print(liluat.render(tmpl, manifest))