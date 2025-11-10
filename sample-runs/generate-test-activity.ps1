# PowerShell script to generate test messages on local IBM MQ queues
# This will create activity for statistics and accounting collection

param(
    [int]$MessageCount = 50,
    [string]$QueueManager = "MQQM1",
    [string]$Channel = "APP1.SVRCONN",
    [string]$ConnName = "127.0.0.1(5200)",
    [string[]]$Queues = @("APP1.REQ", "APP2.REQ")
)

Write-Host "Generating test activity on IBM MQ..."
Write-Host "Queue Manager: $QueueManager"
Write-Host "Channel: $Channel"
Write-Host "Connection: $ConnName"
Write-Host "Queues: $($Queues -join ', ')"
Write-Host "Messages per queue: $MessageCount"

# Create sample messages using runmqsc commands
$timestamp = Get-Date -Format "yyyy-MM-dd HH:mm:ss"

foreach ($queue in $Queues) {
    Write-Host "`nGenerating activity for queue: $queue"
    
    # Put messages using amqsput (the correct way)
    Write-Host "Putting $MessageCount messages to $queue..."
    try {
        $messages = @()
        for ($i = 1; $i -le $MessageCount; $i++) {
            $messages += "Test message $i for $queue - Generated at $timestamp"
        }
        $messages += ""  # Empty line to terminate amqsput
        
        $putResult = $messages | & "C:\Program Files\IBM\MQ\bin64\amqsput.exe" $queue $QueueManager 2>&1
        
        if ($LASTEXITCODE -eq 0) {
            Write-Host "Successfully put $MessageCount messages to $queue" -ForegroundColor Green
        } else {
            Write-Host "Error putting messages to $queue (Exit code: $LASTEXITCODE)" -ForegroundColor Red
            Write-Host $putResult
        }
    }
    catch {
        Write-Host "Failed to execute amqsput: $($_.Exception.Message)" -ForegroundColor Red
    }
    
    # Get some messages back using amqsget to create GET statistics
    $getCount = [Math]::Floor($MessageCount / 2) # Get half the messages back
    Write-Host "Getting $getCount messages from $queue to create reader statistics..."
    
    try {
        for ($i = 1; $i -le $getCount; $i++) {
            $getResult = & "C:\Program Files\IBM\MQ\bin64\amqsget.exe" $queue $QueueManager 2>&1
            if ($LASTEXITCODE -ne 0) {
                Write-Host "No more messages available in $queue" -ForegroundColor Yellow
                break
            }
        }
        Write-Host "Successfully retrieved messages from $queue" -ForegroundColor Green
    }
    catch {
        Write-Host "Failed to execute amqsget: $($_.Exception.Message)" -ForegroundColor Red
    }
}

# Display queue depths
Write-Host "`nChecking queue depths..."
$depthScript = @"
* Display queue depths
DIS QUEUE('APP1.REQ') CURDEPTH MAXDEPTH
DIS QUEUE('APP2.REQ') CURDEPTH MAXDEPTH
* Display queue statistics
DIS QUEUE('APP1.REQ') GET PUT
DIS QUEUE('APP2.REQ') GET PUT
"@

$depthScriptFile = "sample-runs\check_queues.mqsc"
$depthScript | Out-File -FilePath $depthScriptFile -Encoding ASCII

try {
    Write-Host "`nQueue Status:"
    $result = Get-Content $depthScriptFile | & "C:\Program Files\IBM\MQ\bin64\runmqsc" $QueueManager
    Write-Host $result
}
catch {
    Write-Host "Failed to check queue depths: $($_.Exception.Message)" -ForegroundColor Red
}

Write-Host "`nTest activity generation complete!"
Write-Host "You can now run the IBM MQ collector to gather statistics from this activity."