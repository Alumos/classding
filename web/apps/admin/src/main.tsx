import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { setAuthScope } from '@classding/api';
import { SiteSettingsProvider } from '@classding/ui/components/classding';
import { Toaster } from '@classding/ui/components/ui/sonner';
import { TooltipProvider } from '@classding/ui/components/ui/tooltip';
import { useIsMobile } from '@classding/ui/hooks/use-mobile';
import '@classding/ui/globals.css';
import App from './App';

setAuthScope('admin');
const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: 1, refetchOnWindowFocus: false, staleTime: 15_000 } },
});

function AdminToaster() {
  const isMobile = useIsMobile();
  // Keep success/error feedback clear of the narrow-screen header and navigation.
  return <Toaster position={isMobile ? 'bottom-center' : 'top-right'} richColors closeButton />;
}

createRoot(document.getElementById('root')!).render(
  <SiteSettingsProvider>
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <BrowserRouter basename="/admin">
          <App />
          <AdminToaster />
        </BrowserRouter>
      </TooltipProvider>
    </QueryClientProvider>
  </SiteSettingsProvider>,
);
