import { useState, useEffect } from 'react';
import axios from 'axios';
import { Activity, Server, Database, AlertCircle, Clock, CheckCircle, Send } from 'lucide-react';

const API_URL = 'http://localhost:8081';
// NOTE: In production, this should be an environment variable
const API_KEY = 'secret'; // Hardcoded for dev/demo purposes

function App() {
  const [stats, setStats] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [taskType, setTaskType] = useState('email');
  const [priority, setPriority] = useState(1);
  const [enqueueStatus, setEnqueueStatus] = useState(null);

  const fetchStats = async () => {
    try {
      const response = await axios.get(`${API_URL}/stats`, {
        headers: { 'X-API-Key': API_KEY }
      });
      setStats(response.data);
      setError(null);
    } catch (err) {
      console.error(err);
      setError('Failed to fetch stats');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchStats();
    const interval = setInterval(fetchStats, 2000);
    return () => clearInterval(interval);
  }, []);

  const handleEnqueue = async (e) => {
    e.preventDefault();
    setEnqueueStatus('sending');
    try {
      await axios.post(`${API_URL}/enqueue`, {
        type: taskType,
        priority: parseInt(priority),
        payload: { timestamp: new Date().toISOString() }
      }, {
        headers: { 'X-API-Key': API_KEY }
      });
      setEnqueueStatus('success');
      setTimeout(() => setEnqueueStatus(null), 3000);
      fetchStats();
    } catch (err) {
      console.error(err);
      setEnqueueStatus('error');
    }
  };

  if (loading && !stats) return <div className="flex items-center justify-center h-screen text-white">Loading...</div>;

  return (
    <div className="min-h-screen bg-gray-900 text-white p-8 font-sans">
      <div className="max-w-6xl mx-auto">
        <header className="flex items-center justify-between mb-8">
          <div className="flex items-center gap-3">
            <Activity className="w-8 h-8 text-blue-500" />
            <h1 className="text-3xl font-bold bg-clip-text text-transparent bg-gradient-to-r from-blue-400 to-purple-500">
              GoQueue Dashboard
            </h1>
          </div>
          <div className="flex items-center gap-2">
            <div className={`w-3 h-3 rounded-full ${error ? 'bg-red-500' : 'bg-green-500'} animate-pulse`}></div>
            <span className="text-sm text-gray-400">{error ? 'Disconnected' : 'Connected'}</span>
          </div>
        </header>

        {error && (
          <div className="bg-red-900/50 border border-red-700 text-red-200 p-4 rounded-lg mb-8 flex items-center gap-2">
            <AlertCircle className="w-5 h-5" />
            {error}
          </div>
        )}

        {/* Queue Stats Grid */}
        <div className="grid grid-cols-1 md:grid-cols-3 gap-6 mb-8">
          <StatCard
            title="High Priority"
            count={stats?.['queue:high'] || 0}
            icon={<AlertCircle className="w-6 h-6 text-red-400" />}
            color="border-red-500/30 bg-red-500/10"
          />
          <StatCard
            title="Default Priority"
            count={stats?.['queue:default'] || 0}
            icon={<Server className="w-6 h-6 text-blue-400" />}
            color="border-blue-500/30 bg-blue-500/10"
          />
          <StatCard
            title="Low Priority"
            count={stats?.['queue:low'] || 0}
            icon={<Database className="w-6 h-6 text-green-400" />}
            color="border-green-500/30 bg-green-500/10"
          />
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-6 mb-12">
          <StatCard
            title="Processing"
            count={stats?.['processing_queue'] || 0}
            icon={<Activity className="w-6 h-6 text-yellow-400 animate-spin-slow" />}
            color="border-yellow-500/30 bg-yellow-500/10"
          />
          <StatCard
            title="Delayed / Scheduled"
            count={stats?.['delayed_queue'] || 0}
            icon={<Clock className="w-6 h-6 text-purple-400" />}
            color="border-purple-500/30 bg-purple-500/10"
          />
          <StatCard
            title="Dead Letter Queue"
            count={stats?.['dead_letter_queue'] || 0}
            icon={<AlertCircle className="w-6 h-6 text-gray-400" />}
            color="border-gray-500/30 bg-gray-500/10"
          />
        </div>

        {/* Enqueue Form */}
        <div className="bg-gray-800/50 border border-gray-700 rounded-xl p-6 backdrop-blur-sm">
          <h1 className="text-3xl font-bold flex items-center gap-3">
            <Database className="w-8 h-8 text-blue-400 animate-pulse" />
            DistributedQ Dashboard
          </h1>
          <form onSubmit={handleEnqueue} className="flex flex-col md:flex-row gap-4 items-end">
            <div className="flex-1 w-full">
              <label className="block text-sm text-gray-400 mb-1">Task Type</label>
              <select
                value={taskType}
                onChange={(e) => setTaskType(e.target.value)}
                className="w-full bg-gray-900 border border-gray-700 rounded-lg p-2.5 text-white focus:ring-2 focus:ring-blue-500 focus:border-transparent outline-none transition-all"
              >
                <option value="email">Email</option>
                <option value="slow">Slow Task (5s)</option>
                <option value="notification">Notification</option>
                <option value="image_resize">Image Resize</option>
                <option value="report">Report Generation</option>
              </select>
            </div>
            <div className="flex-1 w-full">
              <label className="block text-sm text-gray-400 mb-1">Priority</label>
              <select
                value={priority}
                onChange={(e) => setPriority(e.target.value)}
                className="w-full bg-gray-900 border border-gray-700 rounded-lg p-2.5 text-white focus:ring-2 focus:ring-blue-500 focus:border-transparent outline-none transition-all"
              >
                <option value="2">High</option>
                <option value="1">Default</option>
                <option value="0">Low</option>
              </select>
            </div>
            <button
              type="submit"
              disabled={enqueueStatus === 'sending'}
              className={`px-6 py-2.5 rounded-lg font-medium transition-all flex items-center gap-2 ${enqueueStatus === 'success'
                ? 'bg-green-600 hover:bg-green-700'
                : 'bg-blue-600 hover:bg-blue-700'
                } disabled:opacity-50 disabled:cursor-not-allowed`}
            >
              {enqueueStatus === 'sending' ? (
                'Sending...'
              ) : enqueueStatus === 'success' ? (
                <>
                  <CheckCircle className="w-4 h-4" /> Sent!
                </>
              ) : (
                <>
                  <Send className="w-4 h-4" /> Enqueue Task
                </>
              )}
            </button>
          </form>
        </div>

        {/* Task Inspector Component */}
        <TaskInspector />

      </div>
    </div>
  );
}

function StatCard({ title, count, icon, color }) {
  return (
    <div className={`border rounded-xl p-6 backdrop-blur-sm transition-all hover:scale-105 ${color}`}>
      <div className="flex items-center justify-between mb-4">
        <h3 className="text-gray-400 font-medium">{title}</h3>
        {icon}
      </div>
      <div className="text-4xl font-bold text-white">
        {count}
      </div>
    </div>
  );
}

function TaskInspector() {
  const [tasks, setTasks] = useState([]);
  const [selectedQueue, setSelectedQueue] = useState('queue:default');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [filter, setFilter] = useState('');

  const queues = [
    'queue:high',
    'queue:default',
    'queue:low',
    'processing_queue',
    'delayed_queue',
    'dead_letter_queue',
    'completed_queue'
  ];

  const fetchTasks = async () => {
    setLoading(true);
    try {
      const response = await axios.get(`${API_URL}/tasks?queue=${selectedQueue}`, {
        headers: { 'X-API-Key': API_KEY }
      });
      setTasks(response.data || []);
      setError(null);
    } catch (err) {
      console.error(err);
      setError('Failed to fetch tasks');
      setTasks([]);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchTasks();
  }, [selectedQueue]);

  const filteredTasks = tasks.filter(task =>
    task.id.toLowerCase().includes(filter.toLowerCase()) ||
    task.type.toLowerCase().includes(filter.toLowerCase()) ||
    JSON.stringify(task.payload).toLowerCase().includes(filter.toLowerCase())
  );

  return (
    <div className="bg-gray-800/50 border border-gray-700 rounded-xl p-6 backdrop-blur-sm mt-8">
      <div className="flex flex-col md:flex-row justify-between items-start md:items-center mb-6 gap-4">
        <h2 className="text-xl font-semibold flex items-center gap-2">
          <Database className="w-5 h-5 text-purple-400" />
          Task Inspector
        </h2>
        <div className="flex flex-col md:flex-row gap-2 w-full md:w-auto">
          <select
            value={selectedQueue}
            onChange={(e) => setSelectedQueue(e.target.value)}
            className="bg-gray-900 border border-gray-700 rounded-lg p-2 text-white outline-none focus:border-blue-500"
          >
            {queues.map(q => <option key={q} value={q}>{q}</option>)}
          </select>
          <input
            type="text"
            placeholder="Filter tasks..."
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            className="bg-gray-900 border border-gray-700 rounded-lg p-2 text-white outline-none focus:border-blue-500"
          />
          <button
            onClick={fetchTasks}
            className="p-2 bg-gray-700 hover:bg-gray-600 rounded-lg text-white transition-colors"
            title="Refresh"
          >
            <Activity className={`w-5 h-5 ${loading ? 'animate-spin' : ''}`} />
          </button>
        </div>
      </div>

      <div className="overflow-x-auto">
        <table className="w-full text-left text-sm text-gray-400">
          <thead className="bg-gray-900/50 text-gray-200 uppercase font-medium">
            <tr>
              <th className="p-3 rounded-tl-lg">ID</th>
              <th className="p-3">Type</th>
              <th className="p-3">Priority</th>
              <th className="p-3">Created</th>
              <th className="p-3">Retry</th>
              <th className="p-3 rounded-tr-lg">Payload</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-700">
            {filteredTasks.length === 0 ? (
              <tr>
                <td colSpan="6" className="p-4 text-center text-gray-500">
                  {loading ? 'Loading...' : 'No tasks found in this queue'}
                </td>
              </tr>
            ) : (
              filteredTasks.map(task => (
                <tr key={task.id} className={`transition-colors ${selectedQueue === 'completed_queue'
                  ? 'hover:bg-green-900/20 text-gray-500 italic border-l-2 border-green-500'
                  : 'hover:bg-gray-700/30'
                  }`}>
                  <td className={`p-3 font-mono text-xs ${selectedQueue === 'completed_queue' ? 'text-green-400' : 'text-blue-400'
                    }`}>{task.id.slice(0, 8)}...</td>
                  <td className="p-3">
                    <span className="px-2 py-1 rounded-full bg-gray-700 text-gray-300 text-xs">
                      {task.type}
                    </span>
                  </td>
                  <td className="p-3">
                    <span className={`px-2 py-1 rounded-full text-xs font-bold ${task.priority === 2 ? 'bg-red-900 text-red-300' :
                      task.priority === 0 ? 'bg-green-900 text-green-300' :
                        'bg-blue-900 text-blue-300'
                      }`}>
                      {task.priority === 2 ? 'HIGH' : task.priority === 0 ? 'LOW' : 'DEF'}
                    </span>
                  </td>
                  <td className="p-3 text-xs w-32">
                    {new Date(task.created_at).toLocaleTimeString()}
                  </td>
                  <td className="p-3">{task.retry_count}</td>
                  <td className="p-3 font-mono text-xs max-w-xs truncate" title={JSON.stringify(task.payload)}>
                    {JSON.stringify(task.payload)}
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

export default App;
