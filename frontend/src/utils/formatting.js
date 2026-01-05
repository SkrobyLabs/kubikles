export const formatAge = (timestamp) => {
    if (!timestamp) return '';
    const start = new Date(timestamp).getTime();
    const now = new Date().getTime();
    const diff = Math.floor((now - start) / 1000); // seconds

    if (diff < 60) return `${diff}s`;

    const minutes = Math.floor(diff / 60);
    if (minutes < 60) return `${minutes}m`;

    const hours = Math.floor(minutes / 60);
    const remainingMinutes = minutes % 60;
    if (hours < 24) {
        return remainingMinutes > 0 ? `${hours}h ${remainingMinutes}m` : `${hours}h`;
    }

    const days = Math.floor(hours / 24);
    const remainingHours = hours % 24;

    if (days < 365) {
        return remainingHours > 0 ? `${days}d ${remainingHours}h` : `${days}d`;
    }

    const years = Math.floor(days / 365);
    const remainingDays = days % 365;
    return remainingDays > 0 ? `${years}y ${remainingDays}d` : `${years}y`;
};

export const formatBytes = (bytes) => {
    if (bytes === 0 || bytes == null) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(Math.abs(bytes)) / Math.log(k));
    const value = bytes / Math.pow(k, i);
    return `${value.toFixed(i > 0 ? 1 : 0)} ${sizes[Math.min(i, sizes.length - 1)]}`;
};
