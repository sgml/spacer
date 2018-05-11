local function load (module_name)
    if module_name == 'service_endpoints' then
        return nil
    end

    local service_endpoints = require('service_endpoints')

    if service_endpoints[module_name] == nil then
        return module_name .. " doesn't exist in service endpoints configuration"
    end

    return service_proxy(service_endpoints[module_name])
end

function service_proxy(endpoint)
    return function ()
        local _M = {}
        _M.request = function ()
            return 'calling' .. endpoint
        end

        return _M
    end
end

return function ()
    table.insert(package.loaders, load)
end
