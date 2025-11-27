# PowerShell script to generate test messages on local IBM MQ queues
# This will create activity for statistics and accounting collection

param(
    [int]$MessageCount = 5,
    [string]$QueueManager = "MQQM1",
    [string]$Channel = "APP1.SVRCONN",
    [string]$ConnName = "127.0.0.1(5200)",
    [string[]]$Queues = @("APP1.REQ", "APP2.REQ")
)

Write-Host "Generating test activity on IBM MQ..." -ForegroundColor Cyan
Write-Host "Queue Manager: $QueueManager"
Write-Host "Channel: $Channel"
Write-Host "Connection: $ConnName"
Write-Host "Queues: $($Queues -join ', ')"
Write-Host "Messages per queue: $MessageCount"
Write-Host ""

# Set environment variable for client connection
$env:MQSERVER = "$Channel/TCP/$ConnName"

$timestamp = Get-Date -Format "yyyy-MM-dd HH:mm:ss"

foreach ($queue in $Queues) {
    Write-Host "Processing queue: $queue" -ForegroundColor Yellow
    
    # Put messages using amqsput
    Write-Host "  Putting $MessageCount messages..."
    try {
        $messages = ""
        for ($i = 1; $i -le $MessageCount; $i++) {
            $messages += "Test message $i for $queue - $timestamp`n"
        }
        $messages += "`n"  # Empty line to terminate amqsput
        
        $messages | & "C:\Program Files\IBM\MQ\bin64\amqsput.exe" $queue $QueueManager 2>&1 | Out-Null
        
        if ($LASTEXITCODE -eq 0) {
            Write-Host "  Successfully put $MessageCount messages" -ForegroundColor Green
        } else {
            Write-Host "  Error putting messages (Exit code: $LASTEXITCODE)" -ForegroundColor Red
        }
    }
    catch {
        Write-Host "  Failed to execute amqsput: $($_.Exception.Message)" -ForegroundColor Red
    }
    
    # Get some messages back using amqsget to create GET statistics
    $getCount = [Math]::Max(1, [Math]::Floor($MessageCount / 2))
    Write-Host "  Getting $getCount messages to create reader statistics..."
    
    try {
        $retrieved = 0
        for ($i = 1; $i -le $getCount; $i++) {
            $output = & "C:\Program Files\IBM\MQ\bin64\amqsget.exe" $queue $QueueManager 2>&1
            if ($LASTEXITCODE -eq 0 -and $output -notlike "*no more messages*") {
                $retrieved++
            } else {
                break
            }
        }
        if ($retrieved -gt 0) {
            Write-Host "  Retrieved $retrieved messages" -ForegroundColor Green
        } else {
            Write-Host "  No messages retrieved" -ForegroundColor Yellow
        }
    }
    catch {
        Write-Host "  Failed to execute amqsget: $($_.Exception.Message)" -ForegroundColor Red
    }
    Write-Host ""
}

# Display queue depths
Write-Host "Checking queue depths..." -ForegroundColor Cyan
$depthScript = ""
foreach ($queue in $Queues) {
    $depthScript += "DISPLAY QLOCAL('$queue') CURDEPTH MAXDEPTH`n"
}

try {
    $depthScript | & "C:\Program Files\IBM\MQ\bin64\runmqsc.exe" $QueueManager 2>&1 | Select-String -Pattern "CURDEPTH"
}
catch {
    Write-Host "Failed to check queue depths: $($_.Exception.Message)" -ForegroundColor Red
}

Write-Host ""
Write-Host "Test activity generation complete!" -ForegroundColor Green
Write-Host "You can now run the collector to gather statistics from this activity."