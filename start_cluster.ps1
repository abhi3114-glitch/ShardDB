
# ShardDB Startup Script
# 1. Kill any running instances
Stop-Process -Name "shard-node" -ErrorAction SilentlyContinue
Stop-Process -Name "proxy" -ErrorAction SilentlyContinue
Stop-Process -Name "meta-node" -ErrorAction SilentlyContinue

# 2. Rebuild Binaries
Write-Host "Building ShardDB..."
go build -o bin/meta-node.exe ./cmd/meta-node
go build -o bin/shard-node.exe ./cmd/shard-node
go build -o bin/proxy.exe ./cmd/proxy

# 3. Start Meta Node
Write-Host "Starting Meta Node on Port 9000..."
Start-Process -FilePath "./bin/meta-node.exe" -ArgumentList "-port 9000" -RedirectStandardOutput "meta.log" -RedirectStandardError "meta.log" -WindowStyle Hidden

# 4. Start Shard Nodes
Write-Host "Starting Shard Node 1 (Port 9001)..."
Start-Process -FilePath "./bin/shard-node.exe" -ArgumentList "-id 1", "-port 9001", "-cluster 1=http://localhost:9001,2=http://localhost:9002,3=http://localhost:9003" -RedirectStandardOutput "shard1.log" -RedirectStandardError "shard1.log" -WindowStyle Hidden

Write-Host "Starting Shard Node 2 (Port 9002)..."
Start-Process -FilePath "./bin/shard-node.exe" -ArgumentList "-id 2", "-port 9002", "-cluster 1=http://localhost:9001,2=http://localhost:9002,3=http://localhost:9003" -RedirectStandardOutput "shard2.log" -RedirectStandardError "shard2.log" -WindowStyle Hidden

Write-Host "Starting Shard Node 3 (Port 9003)..."
Start-Process -FilePath "./bin/shard-node.exe" -ArgumentList "-id 3", "-port 9003", "-cluster 1=http://localhost:9001,2=http://localhost:9002,3=http://localhost:9003" -RedirectStandardOutput "shard3.log" -RedirectStandardError "shard3.log" -WindowStyle Hidden

# 5. Start Proxy
Write-Host "Starting Proxy on Port 8080..."
Start-Process -FilePath "./bin/proxy.exe" -ArgumentList "-port 8080", "-meta http://localhost:9000" -RedirectStandardOutput "proxy.log" -RedirectStandardError "proxy.log" -WindowStyle Hidden

Write-Host "Cluster Started! Logs are redirecting to *.log files."
Write-Host "Run 'go run cmd/tools/benchmark/main.go' to test it."
