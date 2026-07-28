import { Routes, Route } from 'react-router-dom';
import Layout from './components/Layout';
import RepoList from './pages/RepoList';
import RepoDetail from './pages/RepoDetail';
import Ask from './pages/Ask';

export default function App() {
  return (
    <Layout>
      <Routes>
        <Route path="/" element={<RepoList />} />
        <Route path="/repos/:id" element={<RepoDetail />} />
        <Route path="/repos/:id/ask" element={<Ask />} />
      </Routes>
    </Layout>
  );
}
