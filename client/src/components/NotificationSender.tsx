import { useState } from 'react';
import { Send, MessageSquare, ChevronDown } from 'lucide-react';
import { format, parseISO } from 'date-fns';
import { cn } from '../lib/utils';
import { Appointment, NotificationLog, NotificationLogDraft, User } from '../types';
import { Button } from './shared';

type NotificationType = 'confirmation' | 'departed' | 'completed';

const NOTIFICATION_TEMPLATES: Record<NotificationType, { label: string; template: string }> = {
  confirmation: {
    label: '預約確認',
    template: '您好 {客戶名}，您的冷氣清洗已預約在 {日期} {時間}，師傅 {師傅名} 將為您服務。',
  },
  departed: {
    label: '師傅出發',
    template: '您好 {客戶名}，師傅 {師傅名} 已出發前往，預計 {時間} 抵達。',
  },
  completed: {
    label: '完工通知',
    template: '您好 {客戶名}，您的冷氣清洗已完成！感謝您的支持。',
  },
};

interface NotificationSenderProps {
  appointment: Appointment;
  technicians: User[];
  notificationLogs: NotificationLog[];
  onSend: (log: NotificationLogDraft) => Promise<NotificationLog | void>;
}

function fillTemplate(template: string, appointment: Appointment, technicians: User[]): string {
  const tech = technicians.find(t => t.id === appointment.technician_id);
  const scheduledDate = appointment.scheduled_at ? format(parseISO(appointment.scheduled_at), 'yyyy/MM/dd') : '';
  const scheduledTime = appointment.scheduled_at ? format(parseISO(appointment.scheduled_at), 'HH:mm') : '';

  return template
    .replace('{客戶名}', appointment.customer_name)
    .replace('{日期}', scheduledDate)
    .replace('{時間}', scheduledTime)
    .replace('{師傅名}', tech?.name || '(未指派)');
}

export default function NotificationSender({ appointment, technicians, notificationLogs, onSend }: NotificationSenderProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [selectedType, setSelectedType] = useState<NotificationType>('confirmation');
  const [channel, setChannel] = useState<'line' | 'sms'>('line');
  const [message, setMessage] = useState('');
  const [isSending, setIsSending] = useState(false);

  const apptLogs = notificationLogs.filter(l => l.appointment_id === appointment.id);

  const handleSelectType = (type: NotificationType) => {
    setSelectedType(type);
    const filled = fillTemplate(NOTIFICATION_TEMPLATES[type].template, appointment, technicians);
    setMessage(filled);
  };

  // handleSend 等待后端写入成功后再关闭面板，避免真实 API 失败时界面误显示“已发送”。
  const handleSend = async () => {
    if (!message.trim() || isSending) return;

    const log: NotificationLogDraft = {
      appointment_id: appointment.id,
      type: channel,
      message: message.trim(),
    };

    setIsSending(true);
    try {
      await onSend(log);
      setIsOpen(false);
      setMessage('');
    } finally {
      setIsSending(false);
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h4 className="text-xs font-bold text-slate-400 uppercase tracking-wider">客戶通知</h4>
        <Button
          variant="outline"
          className="text-xs py-1.5 px-3"
          data-testid="button-open-notification"
          onClick={() => {
            if (!isOpen) {
              handleSelectType('confirmation');
            }
            setIsOpen(!isOpen);
          }}
        >
          <Send className="w-3.5 h-3.5" />
          發送通知
        </Button>
      </div>

      {isOpen && (
        <div className="bg-slate-50 rounded-lg p-5 space-y-4 border border-slate-100">
          <div className="space-y-2">
            <label className="block text-xs font-bold text-slate-500">通知類型</label>
            <div className="flex gap-2 flex-wrap">
              {(Object.keys(NOTIFICATION_TEMPLATES) as NotificationType[]).map(type => (
                <button
                  key={type}
                  type="button"
                  data-testid={`button-notif-type-${type}`}
                  onClick={() => handleSelectType(type)}
                  className={cn(
                    "px-3 py-1.5 rounded-lg text-xs font-medium transition-all",
                    selectedType === type
                      ? "bg-blue-600 text-white"
                      : "bg-white text-slate-600 border border-slate-200 hover:border-slate-300"
                  )}
                >
                  {NOTIFICATION_TEMPLATES[type].label}
                </button>
              ))}
            </div>
          </div>

          <div className="space-y-2">
            <label className="block text-xs font-bold text-slate-500">發送管道</label>
            <div className="flex gap-2">
              <button
                type="button"
                data-testid="button-channel-line"
                onClick={() => setChannel('line')}
                className={cn(
                  "px-3 py-1.5 rounded-lg text-xs font-medium transition-all",
                  channel === 'line'
                    ? "bg-emerald-600 text-white"
                    : "bg-white text-slate-600 border border-slate-200 hover:border-slate-300"
                )}
              >
                LINE
              </button>
              <button
                type="button"
                data-testid="button-channel-sms"
                onClick={() => setChannel('sms')}
                className={cn(
                  "px-3 py-1.5 rounded-lg text-xs font-medium transition-all",
                  channel === 'sms'
                    ? "bg-blue-600 text-white"
                    : "bg-white text-slate-600 border border-slate-200 hover:border-slate-300"
                )}
              >
                簡訊
              </button>
            </div>
          </div>

          <div className="space-y-2">
            <label className="block text-xs font-bold text-slate-500">訊息內容</label>
            <textarea
              data-testid="textarea-notification-message"
              value={message}
              onChange={e => setMessage(e.target.value)}
              rows={4}
              className="w-full px-4 py-3 rounded-md border border-slate-200 text-sm focus:outline-none focus:ring-2 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 resize-none"
            />
          </div>

          <div className="flex gap-2 justify-end">
            <Button
              variant="outline"
              className="text-xs py-1.5 px-3"
              data-testid="button-cancel-notification"
              disabled={isSending}
              onClick={() => { setIsOpen(false); setMessage(''); }}
            >
              取消
            </Button>
            <Button
              variant="primary"
              className="text-xs py-1.5 px-3"
              data-testid="button-send-notification"
              disabled={!message.trim() || isSending}
              onClick={handleSend}
            >
              <Send className="w-3.5 h-3.5" />
              {isSending ? '發送中...' : '發送通知'}
            </Button>
          </div>
        </div>
      )}

      {apptLogs.length > 0 && (
        <div className="space-y-2">
          <p className="text-xs font-bold text-slate-400">發送紀錄</p>
          <div className="space-y-2">
            {apptLogs.map(log => (
              <div key={log.id} className="bg-slate-50 rounded-md p-3 space-y-1 border border-slate-100" data-testid={`notification-log-${log.id}`}>
                <div className="flex items-center justify-between">
                  <span className={cn(
                    "text-[10px] font-bold px-2 py-0.5 rounded-full",
                    log.type === 'line' ? "bg-emerald-100 text-emerald-700" : "bg-blue-100 text-blue-700"
                  )}>
                    {log.type === 'line' ? 'LINE' : '簡訊'}
                  </span>
                  <span className="text-[10px] text-slate-400">
                    {format(parseISO(log.sent_at), 'MM/dd HH:mm')}
                  </span>
                </div>
                <p className="text-xs text-slate-600 leading-relaxed">{log.message}</p>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
