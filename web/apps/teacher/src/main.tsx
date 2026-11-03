import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { setAuthScope } from '@classding/api';
import { SiteSettingsProvider } from '@classding/ui/components/classding';
import { Toaster } from '@classding/ui/components/ui/sonner';
import { TooltipProvider } from '@classding/ui/components/ui/tooltip';
import '@classding/ui/globals.css';
import '@classding/ui/notice.css';
import { AuthProvider } from './auth';
import App from './App';

setAuthScope('teacher');
const queryClient = new QueryClient({ defaultOptions: { queries: { retry: 1, refetchOnWindowFocus: true, staleTime: 15000 } } });
createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <SiteSettingsProvider>
      <QueryClientProvider client={queryClient}>
        <TooltipProvider>
          <BrowserRouter>
            <AuthProvider><App /></AuthProvider>
          </BrowserRouter>
          <Toaster position="top-center" richColors closeButton />
        </TooltipProvider>
      </QueryClientProvider>
    </SiteSettingsProvider>
  </StrictMode>,
);
if (import.meta.env.PROD && 'serviceWorker' in navigator) window.addEventListener('load', () => { void navigator.serviceWorker.register('/sw.js'); });
