import { Modal } from '../../../components/modal';
import { runSpeedtest } from '../../../api/me';
import { formatBytes } from '../../../utils/format';

let speedtestModal: Modal | null = null;
let isTesting = false;

export function initSpeedtestModal() {
  speedtestModal = new Modal('speedtest-modal');

  document.getElementById('btn-speedtest')?.addEventListener('click', () => {
    openSpeedtestModal();
  });

  document.getElementById('btn-retest-speed')?.addEventListener('click', () => {
    startTest();
  });
}

export function openSpeedtestModal() {
  speedtestModal?.open();
  startTest();
}

async function startTest() {
  if (isTesting) return;
  isTesting = true;

  const progressTrack = document.getElementById('speedtest-progress-track');
  const progressFill = document.getElementById('speedtest-progress-fill');
  const statusText = document.getElementById('speedtest-status-text');
  const speedVal = document.getElementById('speedtest-val');
  const speedUnit = document.getElementById('speedtest-unit');
  const speedSub = document.getElementById('speedtest-sub');
  const latencyVal = document.getElementById('speedtest-latency');
  const rateVal = document.getElementById('speedtest-rate');
  const estimateVal = document.getElementById('speedtest-estimate');
  const badgeEl = document.getElementById('speedtest-badge');
  const btnRetest = document.getElementById('btn-retest-speed') as HTMLButtonElement | null;
  const topbarLabel = document.getElementById('speedtest-label');

  if (btnRetest) btnRetest.disabled = true;
  if (progressTrack) progressTrack.style.display = 'block';
  if (progressFill) {
    progressFill.style.width = '35%';
    progressFill.className = 'progress-fill active';
  }
  if (statusText) statusText.textContent = 'Measuring server throughput & latency...';
  if (speedVal) speedVal.textContent = '...';
  if (speedUnit) speedUnit.textContent = 'Mbps';
  if (speedSub) speedSub.textContent = 'Testing connection speed to Disbox server...';
  if (topbarLabel) topbarLabel.textContent = 'Testing...';

  const res = await runSpeedtest();
  isTesting = false;
  if (btnRetest) btnRetest.disabled = false;
  if (progressFill) progressFill.style.width = '100%';

  setTimeout(() => {
    if (progressTrack) progressTrack.style.display = 'none';
  }, 400);

  if (res.success && res.data) {
    const data = res.data;
    const mbps = data.speed_mbps;
    const mbytes = data.speed_mbytes || mbps / 8;
    const latency = data.latency_ms || 5;

    // Display formatted unit (Gbps if >= 1000 Mbps, else Mbps)
    if (mbps >= 1000) {
      if (speedVal) speedVal.textContent = (mbps / 1000).toFixed(2);
      if (speedUnit) speedUnit.textContent = 'Gbps';
    } else {
      if (speedVal) speedVal.textContent = mbps.toFixed(1);
      if (speedUnit) speedUnit.textContent = 'Mbps';
    }

    if (speedSub) {
      speedSub.textContent = `Equivalent to ${mbps.toFixed(1)} Mbps (${formatBytes(mbytes * 1024 * 1024)}/s)`;
    }

    if (topbarLabel) {
      topbarLabel.textContent = mbps >= 1000 ? `${(mbps / 1000).toFixed(2)} Gbps` : `${mbps.toFixed(0)} Mbps`;
    }

    if (statusText) statusText.textContent = 'Test completed successfully';
    if (latencyVal) latencyVal.textContent = `${latency} ms`;
    if (rateVal) rateVal.textContent = `${mbytes.toFixed(1)} MB/s`;

    // Estimate 10GB download time: 10 * 1024 MB / mbytes MB/s
    const seconds10GB = Math.max(1, Math.round((10 * 1024) / Math.max(1, mbytes)));
    const timeFormatted = seconds10GB < 60 ? `~${seconds10GB}s` : `~${Math.round(seconds10GB / 60)} min`;
    if (estimateVal) estimateVal.textContent = timeFormatted;

    if (badgeEl) {
      if (mbps >= 1000) {
        badgeEl.className = 'badge badge-green';
        badgeEl.textContent = '🚀 Ultra-Fast (Gigabit)';
      } else if (mbps >= 300) {
        badgeEl.className = 'badge badge-blue';
        badgeEl.textContent = '⚡ High Speed';
      } else if (mbps >= 100) {
        badgeEl.className = 'badge badge-amber';
        badgeEl.textContent = '🟢 Good Speed';
      } else {
        badgeEl.className = 'badge badge-neutral';
        badgeEl.textContent = '🟡 Moderate';
      }
    }
  } else {
    if (statusText) statusText.textContent = 'Failed to measure speed';
    if (speedVal) speedVal.textContent = 'Error';
    if (speedSub) speedSub.textContent = res.error || 'Connection failed';
    if (topbarLabel) topbarLabel.textContent = 'Speedtest';
  }
}
