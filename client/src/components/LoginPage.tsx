import { useState } from 'react';
import { Package } from 'lucide-react';
import { motion } from 'motion/react';
import { Toaster, toast } from 'react-hot-toast';
import { Button, Card } from './shared';
import { User } from '../types';

interface LoginPageProps {
  onLogin: (phone: string, password: string) => Promise<User>;
}

export default function LoginPage({ onLogin }: LoginPageProps) {
  const [loginForm, setLoginForm] = useState({ phone: '', password: '' });
  const [isSubmitting, setIsSubmitting] = useState(false);

  // handleLogin 统一走后端登录接口，避免继续依赖本地 mock 账号列表。
  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault();
    const phone = loginForm.phone.trim();
    const password = loginForm.password.trim();

    if (!phone || !password) {
      toast.error('請輸入手機號碼與密碼');
      return;
    }

    setIsSubmitting(true);
    try {
      const foundUser = await onLogin(phone, password);
      toast.success(`歡迎回來, ${foundUser.name}`);
    } catch (error) {
      console.error(error);
      toast.error('手機號碼或密碼錯誤');
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="min-h-screen bg-gradient-to-br from-blue-50 via-white to-slate-50 flex items-center justify-center p-6">
      <Toaster position="top-center" />
      <motion.div 
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        className="w-full max-w-sm"
      >
        <div className="text-center mb-8">
          <div className="w-14 h-14 bg-blue-600 rounded-lg flex items-center justify-center mx-auto mb-4 shadow-md shadow-blue-200">
            <Package className="text-white w-7 h-7" />
          </div>
          <h1 className="text-2xl font-bold text-slate-800">CoolDispatch</h1>
          <p className="text-slate-400 text-sm mt-1">冷氣清洗派工系統</p>
        </div>
        
        <Card className="p-8">
          <form onSubmit={handleLogin} className="space-y-5" data-testid="form-login">
            <div>
              <label className="block text-sm font-medium text-slate-600 mb-1.5">手機號碼</label>
              <input 
                data-testid="input-phone"
                type="text" 
                className="w-full px-3 py-2.5 rounded-md border border-slate-300 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500 transition-all text-sm"
                placeholder="請輸入帳號"
                value={loginForm.phone}
                onChange={e => setLoginForm({ ...loginForm, phone: e.target.value })}
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-slate-600 mb-1.5">密碼</label>
              <input 
                data-testid="input-password"
                type="password" 
                className="w-full px-3 py-2.5 rounded-md border border-slate-300 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500 transition-all text-sm"
                placeholder="請輸入密碼"
                value={loginForm.password}
                onChange={e => setLoginForm({ ...loginForm, password: e.target.value })}
              />
            </div>
            <Button data-testid="button-login" type="submit" className="w-full py-3 text-base mt-2" disabled={isSubmitting}>
              {isSubmitting ? '登入中...' : '登入系統'}
            </Button>
          </form>
        </Card>
      </motion.div>
    </div>
  );
}
