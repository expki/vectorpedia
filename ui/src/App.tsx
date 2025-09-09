import { Routes, Route } from 'react-router-dom';
import './App.css';
import { SearchPage } from './pages/Search';
import { Statistics } from './pages/Statistics';

function App() {
  return (
    <Routes>
      <Route path="/" element={<SearchPage />} />
      <Route path="/statistics" element={<Statistics />} />
    </Routes>
  );
}

export default App;