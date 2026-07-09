import React, { useState } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import toast from 'react-hot-toast';
import { useAuth } from '../contexts/AuthContext';
import { PageShell } from '../components/layout/PageShell';
import { Input } from '../components/ui/Input';
import { Button } from '../components/ui/Button';
import { AlertCircle } from 'lucide-react';

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080';

export const LoginPage: React.FC = () => {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  
  const navigate = useNavigate();
  const { login } = useAuth();

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsLoading(true);
    setError(null);

    try {
      // Mock mode fallback
      if (import.meta.env.VITE_USE_MOCK_AUTH === 'true') {
        setTimeout(() => {
          const user = {
            id: 'mock-user-1',
            name: 'QA Tester',
            email: email,
            role: email.includes('owner') ? 'OWNER' : 'CUSTOMER',
            status: 'ACTIVE',
            created_at: new Date().toISOString()
          };
          login('mock-jwt-token', user);
          toast.success('Login berhasil!');
          navigate(user.role === 'OWNER' || user.role === 'STAFF' ? '/owner/dashboard' : '/');
        }, 1000);
        return;
      }

      const response = await fetch(`${API_BASE_URL}/auth/login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, password })
      });

      const data = await response.json();

      if (!response.ok) {
        throw new Error(data.message || data.error || 'Login gagal. Periksa email dan password Anda.');
      }

      login(data.token, data.user);
      toast.success('Login berhasil!');
      navigate(data.user.role === 'OWNER' || data.user.role === 'STAFF' ? '/owner/dashboard' : '/');
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Terjadi kesalahan';
      setError(msg);
      toast.error(msg);
      setIsLoading(false);
    }
  };

  return (
    <PageShell>
      <div className="min-h-[85vh] flex items-center justify-center py-12 px-6">
        <div className="w-full max-w-[900px] bg-surface rounded-3xl shadow-float border border-border-main overflow-hidden flex flex-col md:flex-row">
          
          {/* Left Side - Branding */}
          <div className="md:w-5/12 bg-primary p-12 text-white flex flex-col justify-between relative overflow-hidden hidden md:flex">
            
            <div className="relative z-10">
              <h2 className="text-3xl font-extrabold mb-4 leading-tight">Mulai<br/>Langkah<br/>Juaramu!</h2>
              <p className="text-white/80 font-medium text-sm leading-relaxed">
                Temukan lapangan terbaik, atur jadwal mabar, dan tingkatkan performa olahragamu bersama LapangGo.
              </p>
            </div>
            
            <div className="relative z-10">
              <div className="flex -space-x-3 mb-3">
                {[1,2,3,4].map(i => (
                  <div key={i} className="w-8 h-8 rounded-full border-2 border-white/40 bg-white/20 backdrop-blur-sm flex items-center justify-center text-[10px] font-bold">
                    User
                  </div>
                ))}
              </div>
              <p className="text-xs font-medium text-white/90">Bergabung dengan 10.000+ pengguna lainnya</p>
            </div>
          </div>

          {/* Right Side - Form */}
          <div className="md:w-7/12 p-8 md:p-12 lg:p-16 flex flex-col justify-center bg-surface">
            <div className="mb-8">
              <h1 className="text-3xl font-extrabold tracking-tight text-text-main mb-2">Selamat Datang</h1>
              <p className="text-text-muted font-medium">Masuk untuk melanjutkan ke LapanganGo</p>
            </div>

            {error && (
              <div className="bg-red-50/80 backdrop-blur-sm text-red-600 p-4 rounded-2xl flex items-start gap-3 mb-6 text-[14px] font-bold border border-red-100">
                <AlertCircle className="w-5 h-5 shrink-0 mt-0.5" />
                <p>{error}</p>
              </div>
            )}

            <form onSubmit={handleSubmit} className="space-y-5">
              <Input
                label="Alamat Email"
                type="email"
                placeholder="nama@email.com"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
              />
              <Input
                label="Kata Sandi"
                type="password"
                placeholder="********"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
              />
              
              <div className="pt-2">
                <Button 
                  type="submit" 
                  className="w-full py-3.5 rounded-xl text-base shadow-sm" 
                  isLoading={isLoading}
                >
                  Masuk Sekarang
                </Button>
              </div>
            </form>

            <p className="text-center text-[14px] text-text-muted mt-10 font-medium">
              Belum punya akun? <Link to="/register" className="text-primary font-bold hover:underline transition-all">Daftar di sini</Link>
            </p>
          </div>

        </div>
      </div>
    </PageShell>
  );
};
