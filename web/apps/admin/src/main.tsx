import {setAuthScope} from '@classding/api';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Toaster } from 'sonner';
import '@classding/ui/styles.css';
import './styles.css';
import App from './App';


setAuthScope('admin');
const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: 1, refetchOnWindowFocus: false, staleTime: 15_000 } },
});

createRoot(document.getElementById('root')!).render(
  <QueryClientProvider client={queryClient}>
      <BrowserRouter basename="/admin">
        <App />
        <Toaster position="top-right" richColors />
      </BrowserRouter>
  </QueryClientProvider>,
);
