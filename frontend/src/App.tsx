import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { ClusterProvider } from './context/ClusterContext';
import { FolderProvider } from './context/FolderContext';
import { Home } from './pages/Home';
import { HostsAndClusters } from './pages/HostsAndClusters';
import { VMsAndTemplates } from './pages/VMsAndTemplates';
import { StoragePage } from './pages/Storage';
import { NetworkPage } from './pages/Network';
import { ConsolePage } from './pages/ConsolePage';

function App() {
  return (
    <ClusterProvider>
      <FolderProvider>
        <BrowserRouter>
          <Routes>
            <Route path="/" element={<Home />} />
            <Route path="/hosts" element={<HostsAndClusters />} />
            <Route path="/vms" element={<VMsAndTemplates />} />
            <Route path="/storage" element={<StoragePage />} />
            <Route path="/network" element={<NetworkPage />} />
            <Route path="/console/:type/:vmid/:name" element={<ConsolePage />} />
          </Routes>
        </BrowserRouter>
      </FolderProvider>
    </ClusterProvider>
  );
}

export default App;
