local api = require "gcr.io/spacer-184617/api"

local G = function (args)
    return "Hello from Spacer!" .. api.request()
end

return G
