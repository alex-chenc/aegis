FROM nginx:1.23-alpine

# Copy the pre-built static files to Nginx's web root directory
COPY dist /usr/share/nginx/html

# Copy Nginx configuration file
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
