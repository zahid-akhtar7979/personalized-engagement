FROM node:20-alpine AS builder
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm install
COPY . .
ARG VITE_REPLAY_URL=http://localhost:8081
ARG VITE_ENGAGEMENT_URL=http://localhost:8083
ARG VITE_ANALYTICS_URL=http://localhost:8084
ARG VITE_AI_URL=http://localhost:8085
ARG VITE_WS_URL=ws://localhost:8083
ENV VITE_REPLAY_URL=$VITE_REPLAY_URL
ENV VITE_ENGAGEMENT_URL=$VITE_ENGAGEMENT_URL
ENV VITE_ANALYTICS_URL=$VITE_ANALYTICS_URL
ENV VITE_AI_URL=$VITE_AI_URL
ENV VITE_WS_URL=$VITE_WS_URL
RUN npm run build

FROM nginx:alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/dist /usr/share/nginx/html
EXPOSE 80
