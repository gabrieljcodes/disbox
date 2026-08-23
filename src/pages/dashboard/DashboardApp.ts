import { renderHeader } from './layout/Header';
import { renderNavigation } from './layout/Navigation';
import { renderPanels } from './layout/Panels';
import { renderModals } from './modals/ModalsContainer';

export function renderDashboardApp(): string {
  return `
    <!-- Global Announcements -->
    <div id="announcements-container" class="announcements-wrapper"></div>

    <!-- Top Navigation Bar -->
    ${renderHeader()}

    <!-- Main Container -->
    <main class="dash-container">
      ${renderNavigation()}
      ${renderPanels()}
    </main>

    <!-- Modals Container -->
    ${renderModals()}
  `;
}
