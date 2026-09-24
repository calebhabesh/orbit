import React, { useState, useEffect, useCallback } from 'react';
import {
  FolderRecord,
  PeerListResult,
  WorkStatusResult,
  WorkListResult,
  DoctorReport,
  StorageUsage,
  ConflictsResult,
  SessionInfo,
} from './types';
import { api } from './api';
import { Header } from './components/Header';
import { DoctorModal } from './components/DoctorModal';
import { StorageBanner } from './components/StorageBanner';
import { BootstrapView } from './views/BootstrapView';
import { FoldersView } from './views/FoldersView';
import { FilesView } from './views/FilesView';
import { ConflictsView } from './views/ConflictsView';

export const App: React.FC = () => {
  const [session, setSession] = useState<SessionInfo | null>(null);
  const [checkingSession, setCheckingSession] = useState(true);

  const [currentTab, setCurrentTab] = useState<'folders' | 'files' | 'conflicts'>('folders');
  const [folders, setFolders] = useState<FolderRecord[]>([]);
  const [peersMap, setPeersMap] = useState<Record<string, PeerListResult>>({});
  const [workStatus, setWorkStatus] = useState<WorkStatusResult | null>(null);
  const [workList, setWorkList] = useState<WorkListResult | null>(null);
  const [doctorReport, setDoctorReport] = useState<DoctorReport | null>(null);
  const [storageUsage, setStorageUsage] = useState<StorageUsage | null>(null);
  const [conflictsMap, setConflictsMap] = useState<Record<string, ConflictsResult>>({});
  const [showDoctorModal, setShowDoctorModal] = useState(false);

  // Check session status on mount
  useEffect(() => {
    checkSession();
  }, []);

  const checkSession = async () => {
    try {
      const s = await api.getSession();
      setSession(s);
    } catch {
      setSession({ authenticated: false });
    } finally {
      setCheckingSession(false);
    }
  };

  const loadData = useCallback(async () => {
    if (!session?.authenticated) return;
    try {
      const [fList, wStat, wList, doc, sUsage] = await Promise.all([
        api.getFolders(),
        api.getWorkStatus(),
        api.getWorkList(),
        api.getDoctor(),
        api.getStorageUsage(),
      ]);

      setFolders(fList);
      setWorkStatus(wStat);
      setWorkList(wList);
      setDoctorReport(doc);
      setStorageUsage(sUsage.usage);

      // Fetch peers and conflicts for each folder
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
    } catch (err: any) {
      if (err?.status === 401) {
        setSession({ authenticated: false });
      }
    }
  }, [session?.authenticated]);

  // Initial load when authenticated
  useEffect(() => {
    if (session?.authenticated) {
      loadData();
    }
  }, [session?.authenticated, loadData]);

  // Ordinary polling: every 4 seconds when page is visible
  useEffect(() => {
    if (!session?.authenticated) return;

    const interval = setInterval(() => {
      if (document.visibilityState === 'visible') {
        loadData();
      }
    }, 4000);

    return () => clearInterval(interval);
  }, [session?.authenticated, loadData]);

  const handleLogout = async () => {
    try {
      await api.logout();
    } finally {
      setSession({ authenticated: false });
    }
  };

  if (checkingSession) {
    return (
      <div style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <div className="empty-state">
          <div className="spinner" style={{ width: '2rem', height: '2rem' }} />
          <p>Connecting to local agent…</p>
        </div>
      </div>
    );
  }

  if (!session?.authenticated) {
    return <BootstrapView onAuthenticated={checkSession} />;
  }

  // Calculate total conflicts across all folders
  const totalConflicts = Object.values(conflictsMap).reduce(
    (acc, curr) => acc + (curr?.content_conflicts?.length || 0) + (curr?.structural_conflicts?.length || 0),
    0
  );

  return (
    <div className="app-container">
      <Header
        currentTab={currentTab}
        onTabChange={setCurrentTab}
        conflictCount={totalConflicts}
        doctorReport={doctorReport}
        onOpenDoctor={() => setShowDoctorModal(true)}
        onLogout={handleLogout}
      />

      <StorageBanner
        usage={storageUsage}
        onRunGC={() => {
          if (folders[0]) {
            const fid = folders[0].folder || folders[0].folder_id;
            if (fid) api.runGC(fid).then(loadData);
          }
        }}
      />

      <main>
        {currentTab === 'folders' && (
          <FoldersView
            folders={folders}
            peersMap={peersMap}
            workStatus={workStatus}
            workList={workList}
            storageUsage={storageUsage}
            onRefresh={loadData}
          />
        )}
        {currentTab === 'files' && <FilesView folders={folders} />}
        {currentTab === 'conflicts' && (
          <ConflictsView
            folders={folders}
            conflictsMap={conflictsMap}
            onRefresh={loadData}
          />
        )}
      </main>

      {showDoctorModal && (
        <DoctorModal
          report={doctorReport}
          onClose={() => setShowDoctorModal(false)}
        />
      )}
    </div>
  );
};
