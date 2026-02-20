let statusData = {};

function updateStatus() {
    fetch('/api/status')
        .then(response => response.json())
        .then(data => {
            statusData = data;
            
            if (data.cameras) {
                data.cameras.forEach(cam => {
                    const statusEl = document.querySelector('[data-status="' + cam.name + '"]');
                    const toggleEl = document.querySelector('[data-toggle="' + cam.name + '"]');
                    
                    if (statusEl) {
                        if (cam.connected && cam.streaming) {
                            statusEl.textContent = 'Online';
                            statusEl.className = 'status-badge online';
                        } else if (cam.enabled) {
                            statusEl.textContent = 'Connecting';
                            statusEl.className = 'status-badge connecting';
                        } else {
                            statusEl.textContent = 'Disabled';
                            statusEl.className = 'status-badge offline';
                        }
                    }
                    
                    if (toggleEl) {
                        toggleEl.textContent = cam.running ? 'Stop' : 'Start';
                    }
                });
            }
        })
        .catch(err => {
            console.error('Failed to fetch status:', err);
        });
}

function updateCameraStatus(cameraName) {
    fetch('/api/status/' + encodeURIComponent(cameraName))
        .then(response => response.json())
        .then(data => {
            const statusEl = document.getElementById('rec-status');
            const uptimeEl = document.getElementById('uptime');
            
            if (statusEl) {
                if (data.running) {
                    statusEl.textContent = 'Recording';
                    statusEl.style.color = '#00ff88';
                } else {
                    statusEl.textContent = 'Stopped';
                    statusEl.style.color = '#e94560';
                }
            }
            
            if (uptimeEl) {
                uptimeEl.textContent = data.uptime || '-';
            }
        })
        .catch(err => {
            console.error('Failed to fetch camera status:', err);
        });
}

function loadStorageStats() {
    fetch('/api/storage')
        .then(response => response.json())
        .then(data => {
            const totalSize = document.getElementById('total-size');
            const fileCount = document.getElementById('file-count');
            const retention = document.getElementById('retention');
            const storageInfo = document.getElementById('storage-info');
            
            if (totalSize) totalSize.textContent = data.total_size_human;
            if (fileCount) fileCount.textContent = data.file_count;
            if (retention) retention.textContent = data.retention_days + ' days';
            if (storageInfo) storageInfo.textContent = data.total_size_human + ' (' + data.file_count + ' files)';
            
            const cameraStorageEl = document.getElementById('camera-storage');
            if (cameraStorageEl && data.cameras) {
                let html = '';
                data.cameras.forEach(cam => {
                    html += '<div class="stat-row">';
                    html += '<span class="stat-label">' + cam.name + '</span>';
                    html += '<span>' + cam.size_human + ' (' + cam.file_count + ' files)</span>';
                    html += '</div>';
                });
                cameraStorageEl.innerHTML = html || '<p>No camera data</p>';
            }
        })
        .catch(err => {
            console.error('Failed to fetch storage stats:', err);
        });
}

function startCamera(cameraName) {
    fetch('/api/camera/' + encodeURIComponent(cameraName) + '/start', {
        method: 'POST'
    })
    .then(response => response.json())
    .then(data => {
        if (data.message) {
            updateCameraStatus(cameraName);
            updateStatus();
        } else {
            alert('Error: ' + data.error);
        }
    })
    .catch(err => {
        alert('Failed to start camera: ' + err.message);
    });
}

function stopCamera(cameraName) {
    fetch('/api/camera/' + encodeURIComponent(cameraName) + '/stop', {
        method: 'POST'
    })
    .then(response => response.json())
    .then(data => {
        if (data.message) {
            updateCameraStatus(cameraName);
            updateStatus();
        } else {
            alert('Error: ' + data.error);
        }
    })
    .catch(err => {
        alert('Failed to stop camera: ' + err.message);
    });
}

function toggleCamera(cameraName) {
    const cam = statusData.cameras ? statusData.cameras.find(c => c.name === cameraName) : null;
    if (cam && cam.running) {
        stopCamera(cameraName);
    } else {
        startCamera(cameraName);
    }
}

function refreshRecordings() {
    window.location.reload();
}

function deleteRecording(cameraName, filename) {
    if (!confirm('Are you sure you want to delete ' + filename + '?')) {
        return;
    }
    
    let url = '/recordings/' + encodeURIComponent(filename);
    if (cameraName) {
        url = '/recordings/' + encodeURIComponent(cameraName) + '/' + encodeURIComponent(filename);
    }
    
    fetch(url, {
        method: 'DELETE'
    })
    .then(response => response.json())
    .then(data => {
        if (data.message) {
            alert('Recording deleted');
            refreshRecordings();
        } else {
            alert('Error: ' + data.error);
        }
    })
    .catch(err => {
        alert('Failed to delete recording: ' + err.message);
    });
}
