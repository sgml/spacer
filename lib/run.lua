local json = require "cjson"

local ENV = os.getenv("SPACER_ENV")
local INTERNAL_TOKEN = require "internal_token"

local reject = function (status, body)
    ngx.status = status
    ngx.say(json.encode(body))
    ngx.exit(ngx.HTTP_OK)
end

-- preload body
ngx.req.read_body()


-- normalize uri
-- remove trailing slash
local uri = ngx.var.uri
if string.sub(uri, -1) == '/' then
    uri = string.sub(uri, 0, -2)
end

-- parse request json body, build params
local loader = require 'service_loader'
loader()
local body = ngx.req.get_body_data()
local params = {}
if body then
    params = json.decode(body)
end

local func_path = ngx.var.uri

-- try require given func_path
local ok, ret = pcall(require, func_path)
if not ok then
    -- `ret` will be the error message if error occured
    ngx.log(ngx.ERR, ret)
    local status = nil
    if string.find(ret, "not found") then
        status = 404
    else
        status = 500
    end
    if ENV == 'production' then
        ret = 'Internal Server Error'
    end

    return reject(status, {["error"] = ret})
end

-- call the func
local func = ret
local ok, ret, err = pcall(func, params)
if not ok then
    -- unknown exception thown by error()
    ngx.log(ngx.ERR, ret)
    if ENV == 'production' then
        ret = 'Internal Server Error'
    end
    return reject(500, {["error"] = ret})
end

-- function returned the second result as error
if err then
    ngx.log(ngx.ERR, err)
    return reject(400, {["error"] = err})
end

ngx.say(json.encode({data = ret}))