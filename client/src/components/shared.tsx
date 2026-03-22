import { cn } from '../lib/utils';
import { Appointment } from '../types';

export const Button = ({ className, variant = 'primary', ...props }: React.ButtonHTMLAttributes<HTMLButtonElement> & { variant?: 'primary' | 'secondary' | 'outline' | 'danger' | 'success' }) => {
  const variants = {
    primary: 'bg-blue-600 text-white hover:bg-blue-700 shadow-sm shadow-blue-100',
    secondary: 'bg-slate-100 text-slate-900 hover:bg-slate-200',
    outline: 'border border-slate-200 text-slate-700 hover:bg-slate-50',
    danger: 'bg-rose-500 text-white hover:bg-rose-600 shadow-sm shadow-rose-100',
    success: 'bg-emerald-600 text-white hover:bg-emerald-700 shadow-sm shadow-emerald-100',
  };
  return (
    <button 
      className={cn('px-4 py-2 rounded-md font-medium transition-all active:scale-[0.98] disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-2', variants[variant], className)} 
      {...props} 
    />
  );
};

export const Card = ({ children, className, ...props }: { children: React.ReactNode, className?: string } & React.HTMLAttributes<HTMLDivElement>) => (
  <div className={cn('bg-white border border-slate-200/60 rounded-lg shadow-sm hover:shadow-md transition-shadow duration-300', className)} {...props}>
    {children}
  </div>
);

export const Badge = ({ status }: { status: Appointment['status'] }) => {
  const styles = {
    pending: 'bg-amber-50 text-amber-700 border-amber-200/50',
    assigned: 'bg-blue-50 text-blue-700 border-blue-200/50',
    arrived: 'bg-violet-50 text-violet-700 border-violet-200/50',
    completed: 'bg-emerald-50 text-emerald-700 border-emerald-200/50',
    cancelled: 'bg-rose-50 text-rose-700 border-rose-200/50',
  };
  const labels = {
    pending: '待指派',
    assigned: '已分派',
    arrived: '清洗中',
    completed: '已完成',
    cancelled: '無法清洗',
  };
  return (
    <span data-testid={`badge-status-${status}`} className={cn('px-3 py-1 rounded-full text-xs font-bold border', styles[status])}>
      {labels[status]}
    </span>
  );
};
