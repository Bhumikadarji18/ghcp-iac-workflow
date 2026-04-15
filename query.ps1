# query.ps1 — Send a request to the cost agent and display clean output.
# Usage:
#   .\query.ps1 "show monthly cost breakdown by subscription"
#   .\query.ps1 "check VM CPU rightsizing recommendations"
#   .\query.ps1 "detect idle unused resources"

param(
    [Parameter(Mandatory=$true, Position=0)]
    [string]$Prompt,

    [string]$Url = "http://localhost:8080/agent/cost"
)

$json = '{"messages":[{"role":"user","content":"' + $Prompt.Replace('"','\"') + '"}]}'
$tmpFile = Join-Path $PSScriptRoot "request.json"
[System.IO.File]::WriteAllText($tmpFile, $json)

$raw = curl.exe -s -X POST $Url -H "Content-Type: application/json" --data-binary "@$tmpFile" 2>&1

$output = ""
foreach ($line in $raw -split "`n") {
    $line = $line.Trim()
    if ($line -match '^data:\s*(.+)$') {
        $dataStr = $Matches[1]
        if ($dataStr -eq '{}') { continue }
        try {
            $obj = $dataStr | ConvertFrom-Json
            $content = $obj.choices[0].delta.content
            if ($content) {
                $output += $content
            }
        } catch {
            # skip non-JSON lines
        }
    }
}

Write-Host ""
Write-Host $output
