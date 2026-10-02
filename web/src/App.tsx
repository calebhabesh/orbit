import React, { useState, useEffect, useCallback } from 'react';
import {
  FolderRecord,
  PeerListResult,
  DoctorReport,
  StorageUsage,
  ConflictsResult,
  SessionInfo,
  InspectSetupResult,
  ProductSettings,
} from './types';
import { api } from './api';
import { BootstrapView } from './views/BootstrapView';
import { SetupWizard } from './views/SetupWizard';
import { OrbitSidebar, OrbitNavTab } from './components/OrbitSidebar';
import { OrbitTopbar } from './components/OrbitTopbar';
import { FilesView } from './views/FilesView';
import { NeedsAttentionView } from './views/NeedsAttentionView';
import { DevicesView } from './views/DevicesView';
import { DeletedFilesView } from './views/DeletedFilesView';
import { SettingsView } from './views/SettingsView';

export const App: React.FC = () => {
  // Session & Authentication
  const [session, setSession] = useState<SessionInfo | null>(null);
  const [checkingSession, setCheckingSession] = useState(true);

  // Setup / Onboarding Inspection
  const [setupData, setSetupData] = useState<InspectSetupResult | null>(null);
  const [checkingSetup, setCheckingSetup] = useState(false);

  // Navigation & UI State
  const [currentTab, setCurrentTab] = useState<OrbitNavTab>('files');
  const [searchQuery, setSearchQuery] = useState('');
  const [mobileNavOpen, setMobileNavOpen] = useState(false);

  // Core Data
  const [folders, setFolders] = useState<FolderRecord[]>([]);
  const [peersMap, setPeersMap] = useState<Record<string, PeerListResult>>({});
  const [conflictsMap, setConflictsMap] = useState<Record<string, ConflictsResult>>({});
  const [doctorReport, setDoctorReport] = useState<DoctorReport | null>(null);
  const [storageUsage, setStorageUsage] = useState<StorageUsage | null>(null);
  const [settings, setSettings] = useState<ProductSettings | null>(null);

  // Initial session check
  useEffect(() => {
    checkSession();
  }, []);

  const checkSession = async () => {
    try {
      const s = await api.getSession();
      setSession(s);
      if (s.authenticated) {
        await inspectSetupState();
      }
    } catch {
      setSession({ authenticated: false });
    } finally {
      setCheckingSession(false);
    }
  };

  const inspectSetupState = async () => {
    setCheckingSetup(true);
    try {
      const inspection = await api.inspectSetup();
      setSetupData(inspection);
      if (inspection.settings) {
        setSettings(inspection.settings);
      }
    } catch {
      // Ignore if setup inspect endpoint has issues
    } finally {
      setCheckingSetup(false);
    }
  };

  const loadData = useCallback(async () => {
    if (!session?.authenticated) return;
    try {
      const [fList, doc, sUsage, sett] = await Promise.all([
        api.getFolders(),
        api.getDoctor().catch(() => null),
        api.getStorageUsage().catch(() => null),
        api.getSettings().catch(() => null),
      ]);

      setFolders(fList || []);
      if (doc) setDoctorReport(doc);
      if (sUsage?.usage) setStorageUsage(sUsage.usage);
      if (sett) setSettings(sett);

      // Fetch peers and conflicts for each registered folder
      if (fList && fList.length > 0) {
        const pEntries = await Promise.all(
          fList.map(async (f) => {
            const fid = f.folder || f.folder_id || '';
            if (!fid) return null;
            try {
              const peers = await api.getPeers(fid);
              return [fid, peers] as const;
            } catch {
              return null;
            }
          })
        );
        const newPeers: Record<string, PeerListResult> = {};
        for (const entry of pEntries) {
          if (entry) newPeers[entry[0]] = entry[1];
        }
        setPeersMap(newPeers);

        const cEntries = await Promise.all(
          fList.map(async (f) => {
            const fid = f.folder || f.folder_id || '';
            if (!fid) return null;
            try {
              const conflicts = await api.getConflicts(fid);
              return [fid, conflicts] as const;
            } catch {
              return null;
            }
          })
        );
        const newConflicts: Record<string, ConflictsResult> = {};
        for (const entry of cEntries) {
          if (entry) newConflicts[entry[0]] = entry[1];
        }
        setConflictsMap(newConflicts);
      }
    } catch (err: any) {
      if (err?.status === 401) {
        setSession({ authenticated: false });
      }
    }
  }, [session?.authenticated]);

  // Load data when authenticated and setup completed
  useEffect(() => {
    if (session?.authenticated && setupData && !needsOnboarding(setupData, folders)) {
      loadData();
    }
  }, [session?.authenticated, setupData, loadData]);

  // Periodic polling: every 4 seconds when page is visible
  useEffect(() => {
    if (!session?.authenticated) return;
    if (setupData && needsOnboarding(setupData, folders)) return;

    const interval = setInterval(() => {
      if (document.visibilityState === 'visible') {
        loadData();
      }
    }, 4000);

    return () => clearInterval(interval);
  }, [session?.authenticated, setupData, folders, loadData]);

  const handleLogout = async () => {
    try {
      await api.logout();
    } finally {
      setSession({ authenticated: false });
      setSetupData(null);
    }
  };

  // Helper to determine whether onboarding wizard is required
  function needsOnboarding(data: InspectSetupResult | null, registeredFolders: FolderRecord[]): boolean {
    if (!data) return false;
    // Needs onboarding if setup is incomplete or no folders registered yet
    if (!data.setup_completed) return true;
    if (data.registered_count === 0 && registeredFolders.length === 0) return true;
    return false;
  }

  // 1. Initial Session Loading
  if (checkingSession) {
    return (
      <div style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: 'var(--canvas)' }}>
        <div style={{ textAlign: 'center', color: 'var(--text-secondary)' }}>
          <span className="spinner" style={{ width: '2rem', height: '2rem', border: '2px solid var(--border)', borderTopColor: 'var(--text-primary)', borderRadius: '50%', animation: 'spin 0.8s linear infinite', marginBottom: '0.75rem' }} />
          <div>Connecting to local Orbit agent…</div>
        </div>
      </div>
    );
  }

  // 2. Unauthenticated: Show Bootstrap Token View
  if (!session?.authenticated) {
    return <BootstrapView onAuthenticated={checkSession} />;
  }

  // 3. Inspecting Setup
  if (checkingSetup && !setupData) {
    return (
      <div style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: 'var(--canvas)' }}>
        <div style={{ textAlign: 'center', color: 'var(--text-secondary)' }}>
          <span className="spinner" style={{ width: '2rem', height: '2rem', border: '2px solid var(--border)', borderTopColor: 'var(--text-primary)', borderRadius: '50%', animation: 'spin 0.8s linear infinite', marginBottom: '0.75rem' }} />
          <div>Inspecting workspace state…</div>
        </div>
      </div>
    );
  }

  // 4. Onboarding / First-Device Setup Wizard
  if (setupData && needsOnboarding(setupData, folders)) {
    return (
      <SetupWizard
        inspectData={setupData}
        onComplete={async () => {
          await inspectSetupState();
          await loadData();
          setCurrentTab('files');
        }}
      />
    );
  }

  // 5. Main Orbit Shell
  const totalConflicts = Object.values(conflictsMap).reduce(
    (acc, curr) => acc + (curr?.content_conflicts?.length || 0) + (curr?.structural_conflicts?.length || 0),
    0
  );
  const pausedFolderCount = folders.filter((f) => f.paused).length;
  const doctorWarningCount = (doctorReport?.checks || []).filter((c) => c.status === 'fail' || c.status === 'warn').length;
  const totalAttentionCount = totalConflicts + pausedFolderCount + doctorWarningCount;

  const currentFolder = folders[0];
  const workspaceTitle = settings?.workspace_names?.[currentFolder?.folder || ''] || 'Orbit';

  // Breadcrumbs
  const breadcrumbLabels = [workspaceTitle];
  if (currentTab === 'attention') breadcrumbLabels.push('Needs attention');
  else if (currentTab === 'devices') breadcrumbLabels.push('Devices');
  else if (currentTab === 'deleted') breadcrumbLabels.push('Deleted files');
  else if (currentTab === 'settings') breadcrumbLabels.push('Settings');

  return (
    <div className="orbit-layout">
      {/* Sidebar Navigation */}
      <OrbitSidebar
        currentTab={currentTab}
        onTabChange={setCurrentTab}
        attentionCount={totalAttentionCount}
        settings={settings}
        workspaceName={workspaceTitle}
        isSyncing={false}
        onLogout={handleLogout}
        isOpenMobile={mobileNavOpen}
        onCloseMobile={() => setMobileNavOpen(false)}
      />

      {/* Main Content Area */}
      <div className="orbit-main">
        {/* Topbar */}
        <OrbitTopbar
          breadcrumbs={breadcrumbLabels}
          onNavigateBreadcrumb={(idx) => {
            if (idx === 0) setCurrentTab('files');
          }}
          searchQuery={searchQuery}
          onSearchChange={setSearchQuery}
          onRefresh={loadData}
          currentFolderId={currentFolder?.folder || currentFolder?.folder_id}
          onToggleMobileNav={() => setMobileNavOpen(!mobileNavOpen)}
        />

        {/* View Routing */}
        <main className="orbit-content">
          {currentTab === 'files' && (
            <FilesView
              folders={folders}
              searchQuery={searchQuery}
              onSearchChange={setSearchQuery}
              onOpenFolder={() => {
                if (currentFolder) {
                  api.openLocalFolder(currentFolder.folder || currentFolder.folder_id || '');
                }
              }}
            />
          )}

          {currentTab === 'attention' && (
            <NeedsAttentionView
              folders={folders}
              conflictsMap={conflictsMap}
              doctorReport={doctorReport}
              peersMap={peersMap}
              storageUsage={storageUsage}
              onRefresh={loadData}
            />
          )}

          {currentTab === 'devices' && (
            <DevicesView
              folders={folders}
              peersMap={peersMap}
              settings={settings}
              onRefresh={loadData}
            />
          )}

          {currentTab === 'deleted' && (
            <DeletedFilesView
              folders={folders}
              onRefresh={loadData}
            />
          )}

          {currentTab === 'settings' && (
            <SettingsView
              folders={folders}
              settings={settings}
              storageUsage={storageUsage}
              doctorReport={doctorReport}
              onRefresh={loadData}
              onLogout={handleLogout}
            />
          )}
        </main>
      </div>
    </div>
  );
};
