local want = import "want";

local repo(account, name, commitHash) =
    local url = "https://github.com/%s/%s" % [account, name];
    want.importGit(url, commitHash);

{
    repo :: repo
}